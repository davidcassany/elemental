/*
Copyright © 2025 SUSE LLC
SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package install

import (
	"context"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/selinux"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
	"github.com/suse/elemental/v3/pkg/unpack"
)

const configFile = "config.sh"

type Option func(*Installer)

type Installer struct {
	ctx context.Context
	s   *sys.System
	t   transaction.Interface
	bm  *firmware.EfiBootManager
}

func WithTransaction(t transaction.Interface) Option {
	return func(i *Installer) {
		i.t = t
	}
}

func WithBootManager(bm *firmware.EfiBootManager) Option {
	return func(i *Installer) {
		i.bm = bm
	}
}

func New(ctx context.Context, s *sys.System, opts ...Option) *Installer {
	installer := &Installer{
		s:   s,
		ctx: ctx,
	}
	for _, o := range opts {
		o(installer)
	}
	if installer.t == nil {
		installer.t = transaction.NewSnapperTransaction(ctx, s)
	}
	return installer
}

func (i Installer) Install(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	for _, disk := range d.Disks {
		err = diskrepart.PartitionAndFormatDevice(i.s, disk)
		if err != nil {
			i.s.Logger().Error("installation failed, could not partition '%s'", disk.Device)
			return err
		}
		for _, part := range disk.Partitions {
			err = createPartitionVolumes(i.s, cleanup, part)
			if err != nil {
				i.s.Logger().Error("installation failed, could not create rw volumes")
				return err
			}
		}
	}

	err = i.t.Init(*d)
	if err != nil {
		i.s.Logger().Error("installation failed, could not initialize snapper")
		return err
	}

	trans, err := i.t.Start()
	if err != nil {
		i.s.Logger().Error("installation failed, could not start snapper transaction")
		return err
	}
	cleanup.PushErrorOnly(func() error { return i.t.Rollback(trans, err) })

	err = i.t.Update(trans, d.SourceOS, i.transactionHook(d, trans.Path))
	if err != nil {
		i.s.Logger().Error("installation failed, could not update transaction")
		return err
	}

	if d.OverlayTree != nil && !d.OverlayTree.IsEmpty() {
		unpacker, err := unpack.NewUnpacker(
			i.s, d.OverlayTree, unpack.WithRsyncFlags(overlayTreeSyncFlags()...),
		)
		if err != nil {
			i.s.Logger().Error("installation failed, could not initialize unpacker")
			return err
		}
		_, err = unpacker.Unpack(i.ctx, trans.Path)
		if err != nil {
			i.s.Logger().Error("installation failed, could not unpack overlay tree")
			return err
		}
	}

	if d.CfgScript != "" {
		err = i.configHook(d.CfgScript, trans.Path)
		if err != nil {
			i.s.Logger().Error("installation failed, configuration hook error")
			return err
		}
	}

	err = i.t.Commit(trans)
	if err != nil {
		i.s.Logger().Error("installation failed, could not close transaction")
		return err
	}

	return nil
}

func createPartitionVolumes(s *sys.System, cleanStack *cleanstack.CleanStack, part *deployment.Partition) (err error) {
	var mountPoint string

	if len(part.RWVolumes) > 0 || part.Role == deployment.System {
		mountPoint, err = vfs.TempDir(s.FS(), "", "elemental_"+part.Role.String())
		if err != nil {
			s.Logger().Error("failed creating temporary directory to mount system partition")
			return err
		}
		cleanStack.PushSuccessOnly(func() error { return s.FS().RemoveAll(mountPoint) })

		bDev := lsblk.NewLsDevice(s)
		bPart, err := block.GetPartitionByUUID(s, bDev, part.UUID, 4)
		if err != nil {
			s.Logger().Error("failed to find partition %d", part.UUID)
			return err
		}
		err = s.Mounter().Mount(bPart.Path, mountPoint, "", []string{})
		if err != nil {
			return err
		}
		cleanStack.Push(func() error { return s.Mounter().Unmount(mountPoint) })

		err = btrfs.SetBtrfsPartition(s, mountPoint)
		if err != nil {
			s.Logger().Error("failed setting brfs partition volumes")
			return err
		}
	}

	for _, rwVol := range part.RWVolumes {
		if rwVol.Snapshotted {
			continue
		}
		subvolume := filepath.Join(mountPoint, btrfs.TopSubVol, rwVol.Path)
		err = btrfs.CreateSubvolume(s, subvolume, true)
		if err != nil {
			s.Logger().Error("failed creating subvolume %s", subvolume)
			return err
		}
	}

	return nil
}

func (i Installer) transactionHook(d *deployment.Deployment, root string) transaction.UpdateHook {
	return func() error {
		err := selinux.ChrootedRelabel(i.ctx, i.s, root, nil)
		if err != nil {
			i.s.Logger().Error("failed relabelling snapshot path: %s", root)
			return err
		}

		err = d.WriteDeploymentFile(i.s, root)
		if err != nil {
			i.s.Logger().Error("installation failed, could not write deployment file")
			return err
		}
		return nil
	}
}

func (i Installer) configHook(config string, root string) error {
	i.s.Logger().Info("Running transaction hook")
	rootedConfig := filepath.Join("/etc/elemental", configFile)
	callback := func() error {
		var stdOut, stdErr *string
		stdOut = new(string)
		stdErr = new(string)
		defer func() {
			logOutput(i.s, *stdOut, *stdErr)
		}()
		return i.s.Runner().RunContextParseOutput(i.ctx, stdHander(stdOut), stdHander(stdErr), rootedConfig)
	}
	binds := map[string]string{config: rootedConfig}
	return chroot.ChrootedCallback(i.s, root, binds, callback)
}

func stdHander(out *string) func(string) {
	return func(line string) {
		*out += line + "\n"
	}
}

func logOutput(s *sys.System, stdOut, stdErr string) {
	output := "------- stdOut -------\n"
	output += stdOut
	output += "------- stdErr -------\n"
	output += stdErr
	output += "----------------------\n"
	s.Logger().Debug("Install config hook output:\n%s", output)
}

// overlayTreeSyncFlags provides the rsync flags that are used to sync directories or raw images
// during the overlay tree extraction. It does not keep permissions on pre-existing files or
// directories and it does not keep ownership of files and directories.
func overlayTreeSyncFlags() []string {
	return []string{
		"--recursive",
		"--hard-links",
		"--links",
		"--info=progress2",
		"--human-readable",
		"--partial",
	}
}
