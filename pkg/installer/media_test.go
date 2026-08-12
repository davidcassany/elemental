/*
Copyright © 2025-2026 SUSE LLC
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

package installer_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestInstallerMediaSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "InstallerMedia test suite")
}

var _ = Describe("InstallerMedia", Label("installermedia"), func() {
	var runner *sysmock.Runner
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var d *deployment.Deployment

	var sideEffects map[string]func(...string) ([]byte, error)
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		sideEffects = map[string]func(...string) ([]byte, error){}
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/dev/device":  []byte{},
			"/dev/device1": []byte{},
			"/dev/device2": []byte{},
		})
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		d = deployment.DefaultDeployment()
		d.Installer = deployment.LiveInstaller{}
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if f := sideEffects[cmd]; f != nil {
				return f(args...)
			}
			return runner.ReturnValue, runner.ReturnError
		}
		Expect(vfs.MkdirAll(fs, "/some/dir", vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("Creates an installation ISO", func() {
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			Expect(fs.WriteFile("/some/dir/build/installer.iso", []byte("data"), vfs.FilePerm)).To(Succeed())
			return []byte{}, nil
		}

		d.SourceOS = deployment.NewDirSrc("/some/root")
		d.Installer.OverlayTree = deployment.NewDirSrc("/some/dir/iso-overlay")
		d.Installer.CfgScript = "/some/dir/config-live.sh"
		d.Installer.KernelCmdline = "console=ttyS0"

		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))

		iso.OutputDir = "/some/dir/build"
		d.CfgScript = "/some/dir/config.sh"
		d.OverlayTree = deployment.NewDirSrc("/some/dir/install-overlay")

		Expect(vfs.MkdirAll(fs, "/some/dir/iso-overlay", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(fs, "/some/dir/install-overlay", vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile("/some/dir/config-live.sh", []byte("live config script"), vfs.FilePerm)).To(Succeed())
		Expect(fs.WriteFile("/some/dir/config.sh", []byte("install config script"), vfs.FilePerm)).To(Succeed())

		Expect(iso.Build(d)).To(Succeed())
		Expect(runner.MatchMilestones([][]string{
			{"mksquashfs", "/some/dir/build/elemental-installer/rootfs", "/some/dir/build/elemental-installer/iso/LiveOS/squashfs.img"},
			{"mkfs.vfat", "-n", "EFI", "/some/dir/build/elemental-installer/efi.img"},
			{"mcopy", "-s", "-i", "/some/dir/build/elemental-installer/efi.img", "/some/dir/build/elemental-installer/efi/EFI", "::"},
			{"xorriso", "-volid", "LIVE", "-padding", "0", "-outdev", "/some/dir/build/installer.iso"},
		}))
	})
	It("preserves source provenance in generated install description when rewriting OS source to squashfs", func() {
		Expect(vfs.MkdirAll(fs, "/source", vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile("/source/os.raw", []byte("os"), vfs.FilePerm)).To(Succeed())
		d.SourceOS = deployment.NewRawSrc("/source/os.raw")
		provenance := deployment.NewOCISrc("registry.example.com/elemental-os:1.2.3")
		provenance.SetDigest("sha256:osimage")
		d.SourceOS.SetProvenance(provenance)

		media := installer.NewMedia(context.Background(), s, installer.Disk, installer.WithBootloader(bootloader.NewNone(s)))

		Expect(media.PrepareInstallerFS("/some/dir/live", "/some/dir/work", d)).To(Succeed())

		data, err := fs.ReadFile("/some/dir/live/Install/install.yaml")
		Expect(err).ToNot(HaveOccurred())
		written := &deployment.Deployment{}
		Expect(yaml.Unmarshal(data, written)).To(Succeed())
		Expect(written.SourceOS).NotTo(BeNil())
		Expect(written.SourceOS.String()).To(Equal("raw:///run/initramfs/live/LiveOS/squashfs.img"))
		Expect(written.SourceOS.Provenance()).NotTo(BeNil())
		Expect(written.SourceOS.Provenance().String()).To(Equal("oci://registry.example.com/elemental-os:1.2.3"))
		Expect(written.SourceOS.Provenance().GetDigest()).To(Equal("sha256:osimage"))
	})
	It("fails to create an ISO without an output directory defined", func() {
		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))

		err := iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not compute image checksum"))
	})
	It("fails to create an ISO on a readonly FS", func() {
		roFS, err := sysmock.ReadOnlyTestFS(fs)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(roFS),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())

		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))
		iso.OutputDir = "/some/dir/build"

		err = iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("operation not permitted"))
	})
	It("fails to sync OS content", func() {
		sideEffects["rsync"] = func(args ...string) ([]byte, error) {
			return []byte{}, fmt.Errorf("rsync command failed")
		}

		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))
		iso.OutputDir = "/some/dir/build"

		err := iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rsync command failed"))
	})
	It("fails to burn ISO", func() {
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			return []byte{}, fmt.Errorf("xorriso command failed")
		}

		d.SourceOS = deployment.NewDirSrc("/some/root")
		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))
		iso.OutputDir = "/some/dir/build"

		err := iso.Build(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("xorriso command failed"))
	})
	It("customizes an ISO", func() {
		Expect(vfs.MkdirAll(fs, "/some/dir/build", vfs.DirPerm)).To(Succeed())

		// Create the file pointed out by -outdev when xorriso is called.
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			offset := 0
			for i, arg := range args {
				switch arg {
				case "-outdev":
					offset = 1
				case "-extract":
					offset = 2
				default:
					continue
				}

				file := args[i+offset]
				_, err := fs.Create(file)
				Expect(err).To(Succeed())

				break
			}
			return []byte{}, nil
		}
		sideEffects["grub2-editenv"] = func(args ...string) ([]byte, error) {
			path := args[0]
			if args[1] == "set" {
				Expect(fs.WriteFile(path, []byte(strings.Join(args[2:], "\n")), vfs.FilePerm)).To(Succeed())
			}
			return []byte{}, nil
		}

		_, err := fs.Create("/some/dir/installer.iso")
		Expect(err).To(Succeed())

		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))
		iso.InputFile = "/some/dir/installer.iso"
		iso.OutputDir = "/some/dir/build"
		iso.Name = "installer2"

		Expect(iso.Customize(d)).To(Succeed())

		Expect(vfs.Exists(fs, "/some/dir/build/installer2.iso")).To(BeTrue())
	})
	It("customizes a raw disk with all merged deployment partitions", func() {
		Expect(vfs.MkdirAll(fs, "/some/dir/build", vfs.DirPerm)).To(Succeed())

		installDesc := deployment.New(deployment.WithRecoveryPartition(512))
		installDesc.SourceOS = deployment.NewRawSrc("/run/initramfs/live/LiveOS/squashfs.img")
		installData, err := yaml.Marshal(installDesc)
		Expect(err).ToNot(HaveOccurred())

		d = deployment.New(
			deployment.WithRecoveryPartition(512),
			deployment.WithConfigPartition(128),
		)
		d.Installer = deployment.LiveInstaller{}

		seenLabels := []string{}
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			for i, arg := range args {
				if arg != "-extract" {
					continue
				}
				src := args[i+1]
				dst := args[i+2]
				switch src {
				case "Install/install.yaml":
					Expect(vfs.MkdirAll(fs, filepath.Dir(dst), vfs.DirPerm)).To(Succeed())
					Expect(fs.WriteFile(dst, installData, vfs.FilePerm)).To(Succeed())
				case "/":
					Expect(vfs.MkdirAll(fs, filepath.Join(dst, "boot"), vfs.DirPerm)).To(Succeed())
					Expect(vfs.MkdirAll(fs, filepath.Join(dst, "EFI"), vfs.DirPerm)).To(Succeed())
				}
				break
			}
			return []byte{}, nil
		}
		sideEffects["grub2-editenv"] = func(args ...string) ([]byte, error) {
			Expect(fs.WriteFile(args[0], []byte(strings.Join(args[2:], "\n")), vfs.FilePerm)).To(Succeed())
			return []byte{}, nil
		}
		sideEffects["rsync"] = func(args ...string) ([]byte, error) {
			return []byte{}, nil
		}
		sideEffects["systemd-repart"] = func(args ...string) ([]byte, error) {
			Expect(fs.WriteFile("/some/dir/build/installer.raw", []byte("raw"), vfs.FilePerm)).To(Succeed())
			for _, arg := range args {
				if !strings.HasPrefix(arg, "--definitions=") {
					continue
				}
				defsDir := strings.TrimPrefix(arg, "--definitions=")
				entries, err := fs.ReadDir(defsDir)
				Expect(err).ToNot(HaveOccurred())
				for _, entry := range entries {
					data, err := fs.ReadFile(filepath.Join(defsDir, entry.Name()))
					Expect(err).ToNot(HaveOccurred())
					for _, label := range []string{deployment.EfiLabel, deployment.RecoveryLabel, deployment.ConfigLabel, deployment.SystemLabel} {
						if strings.Contains(string(data), "Label="+label) {
							seenLabels = append(seenLabels, label)
						}
					}
				}
			}
			return []byte("[]"), nil
		}

		_, err = fs.Create("/some/dir/installer.iso")
		Expect(err).ToNot(HaveOccurred())
		raw := installer.NewMedia(
			context.Background(),
			s,
			installer.Disk,
			installer.WithBootloader(bootloader.NewNone(s)),
			installer.WithOutputFile("/some/dir/build/installer.raw"),
		)
		raw.InputFile = "/some/dir/installer.iso"
		raw.OutputDir = "/some/dir/build"

		Expect(raw.Customize(d)).To(Succeed())
		Expect(seenLabels).To(ConsistOf(deployment.EfiLabel, deployment.RecoveryLabel, deployment.ConfigLabel, deployment.SystemLabel))
	})

	It("fails to customize an iso that is not including an install.yaml file", func() {
		Expect(vfs.MkdirAll(fs, "/some/dir/build", vfs.DirPerm)).To(Succeed())

		// Error while extracting Install/install.yaml
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			if slices.Contains(args, "-extract") {
				return nil, fmt.Errorf("fatal error")
			}
			return []byte{}, nil
		}

		_, err := fs.Create("/some/dir/installer.iso")
		Expect(err).To(Succeed())

		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))
		iso.InputFile = "/some/dir/installer.iso"
		iso.OutputDir = "/some/dir/build"
		iso.Name = "installer2"

		Expect(iso.Customize(d)).To(MatchError(ContainSubstring("failed extracting install description")))
	})
	It("fails to customize non-existent input file", func() {
		iso := installer.NewMedia(context.Background(), s, installer.ISO)
		iso.InputFile = "/non-existent/installer.iso"
		iso.OutputDir = "/some/dir/build"
		iso.Name = "installer2.iso"

		err := iso.Customize(d)
		Expect(err).ToNot(Succeed())
		Expect(err.Error()).To(ContainSubstring("target input file /non-existent/installer.iso does not exist"))
	})
	It("fails to customize an ISO using xorriso", func() {
		sideEffects["xorriso"] = func(args ...string) ([]byte, error) {
			return []byte{}, fmt.Errorf("failed to run xorriso")
		}

		_, err := fs.Create("/some/dir/installer.iso")
		Expect(err).To(Succeed())

		iso := installer.NewMedia(context.Background(), s, installer.ISO, installer.WithBootloader(bootloader.NewNone(s)))
		iso.InputFile = "/some/dir/installer.iso"
		iso.OutputDir = "/some/dir/build"
		iso.Name = "installer2"

		err = iso.Customize(d)
		Expect(err).ToNot(Succeed())
		Expect(err.Error()).To(ContainSubstring("failed to run xorriso"))
	})
})
