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

package cpio

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"path/filepath"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// CreateCPIO walks a directory and streams the contents into the given generated CPIO file.
// Requires cpio binary present in the current PATH.
func CreateCPIO(ctx context.Context, s *sys.System, sourceDir, outputPath string) (err error) {
	outFile, err := s.FS().Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer func() {
		e := outFile.Close()
		if err == nil && e != nil {
			err = e
		}
	}()

	callback := func(stdin io.Writer) error {
		return vfs.WalkDirFs(s.FS(), sourceDir, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Extract relative path to maintain directory structure
			relPath, err := filepath.Rel(sourceDir, path)
			if err != nil {
				return err
			}

			// Feed the relative path to cpio, terminated by a null byte (\x00)
			_, writeErr := io.WriteString(stdin, relPath+"\x00")
			return writeErr
		})
	}

	realSource, err := s.FS().RawPath(sourceDir)
	if err != nil {
		return fmt.Errorf("defining the real path for %q: %w", sourceDir, err)
	}

	err = s.Runner().RunContextWithPipe(ctx, callback, outFile, nil, realSource, nil, "cpio", "-0", "-o", "-H", "newc")
	if err != nil {
		return err
	}

	return nil
}

// ExtractCPIO streams the given cpio file to cpio command to extract the contents to the target directory.
// Target directory is created if it does not exist.
func ExtractCPIO(ctx context.Context, s *sys.System, cpioFile, targetDir string) error {
	err := vfs.MkdirAll(s.FS(), targetDir, vfs.DirPerm)
	if err != nil {
		return err
	}

	cpioFile, err = s.FS().RawPath(cpioFile)
	if err != nil {
		return fmt.Errorf("defining the real path for %q: %w", cpioFile, err)
	}

	targetDir, err = s.FS().RawPath(targetDir)
	if err != nil {
		return fmt.Errorf("defining the real path for %q: %w", targetDir, err)
	}

	_, err = s.Runner().RunContext(ctx, "cpio", "-i", "--file", cpioFile, "-d", "-m", "--directory", targetDir)
	return err
}
