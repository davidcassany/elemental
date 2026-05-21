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

package selinux

import (
	"container/ring"
	"context"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	SelinuxTargetedContextFile = selinuxTargetedPath + "/contexts/files/file_contexts"

	selinuxTargetedPath = "/etc/selinux/targeted"
	selinuxAutoRelabel  = "/etc/selinux/.autorelabel"
	debugLines          = 10
)

// SystemRelabel applies the SE Linux labels based on the targeted policy found within the given
// root path. It force applies the labels under the given root except for the given RW paths
// This is to prevent runtime changes during the upgrades as RW paths are potentially in use for current
// processes. If at least one rwPath was provided it also sets the .autorelabel file to trigger
// relabelling at boot and relabel the excluded paths.
func SystemRelabel(ctx context.Context, s *sys.System, rootDir string, rwPaths ...string) error {
	contextFile := filepath.Join(rootDir, SelinuxTargetedContextFile)
	contextExists, _ := vfs.Exists(s.FS(), contextFile)

	if contextExists {
		var err error

		args := []string{"-i"}

		// We only keep last 10 lines of the stdout and stderr for debugging purposes
		stdOut := ring.New(debugLines)
		stdErr := ring.New(debugLines)

		if rootDir == "/" || rootDir == "" {
			rootDir = "/"
		} else {
			args = append(args, "-r", rootDir)
		}

		args = append(args, "-F")
		if len(rwPaths) > 0 {
			for _, rwp := range rwPaths {
				args = append(args, "-e", rwp)
			}
			err = s.FS().WriteFile(filepath.Join(rootDir, selinuxAutoRelabel), []byte{}, vfs.FilePerm)
			if err != nil {
				return fmt.Errorf("creating .autorelabel file: %w", err)
			}
		}
		args = append(args, contextFile, rootDir)

		s.Logger().Info("Applying SE Linux labels to the read-only root tree, forced relabelling")
		err = s.Runner().RunContextParseOutput(ctx, stdHander(stdOut), stdHander(stdErr), "setfiles", args...)
		logOutput(s, stdOut, stdErr)

		return err
	}

	s.Logger().Warn("Not relabelling SE Linux, no context found")
	return nil
}

// ChrootedSystemRelabel applies the SE Linux labels based on the targeted policy found within the given
// root path. Runs the same logic as RelabelSystem method but running inside a chroot environment.
func ChrootedSystemRelabel(ctx context.Context, s *sys.System, rootDir string, rwPaths ...string) error {
	callback := func() error { return SystemRelabel(ctx, s, "/", rwPaths...) }
	err := chroot.ChrootedCallback(s, rootDir, nil, callback, chroot.WithoutDefaultBinds())
	if err != nil {
		return fmt.Errorf("chrooted system relabel: %w", err)
	}
	return nil
}

func stdHander(r *ring.Ring) func(string) {
	return func(line string) {
		r.Value = line
		r = r.Next()
	}
}

func logOutput(s *sys.System, stdOut, stdErr *ring.Ring) {
	output := "\n------- stdOut -------\n"
	stdOut.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	output += "------- stdErr -------\n"
	stdErr.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	output += "----------------------\n"
	s.Logger().Debug("SE Linux setfile call stdout: %s", output)
}
