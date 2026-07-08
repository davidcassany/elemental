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

package cpio_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/internal/cpio"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestCPIOSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CPIO test suite")
}

var _ = Describe("CPIO files creator with mock runner", Label("cpio"), func() {
	var tfs vfs.FS
	var runner *sysmock.Runner
	var cleanup func()
	var err error
	var s *sys.System

	BeforeEach(func() {
		tfs, cleanup, err = sysmock.TestFS(map[string]any{
			"/some/root/file1":        []byte("file1"),
			"/some/root/file2":        []byte("file2"),
			"/some/root/subdir/file3": []byte("file3"),
		})
		Expect(err).NotTo(HaveOccurred())
		runner = sysmock.NewRunner()

		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithRunner(runner),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("creates a CPIO file with the given root", func() {
		Expect(vfs.MkdirAll(tfs, "/output", vfs.DirPerm)).To(Succeed())

		Expect(cpio.CreateCPIO(context.Background(), s, "/some/root", "/output/cpiofile")).To(Succeed())

		data, err := tfs.ReadFile("/output/cpiofile")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(".\x00file1\x00file2\x00subdir\x00subdir/file3\x00"))
	})

	It("extracts a CPIO file to the given target", func() {
		Expect(cpio.ExtractCPIO(context.Background(), s, "/tmp/someFile", "/tmp/extraction")).To(Succeed())
		exists, _ := vfs.Exists(s.FS(), "/tmp/extraction")
		Expect(exists).To(BeTrue())
		Expect(runner.CmdsMatch([][]string{{"cpio", "-i", "--file"}})).To(Succeed())
	})
})

// This test assumes cpio command to be available in PATH, it actually calls the real command
var _ = Describe("CPIO files creator with real runner", Label("cpio"), func() {
	var tfs vfs.FS
	var cleanup func()
	var err error
	var s *sys.System

	BeforeEach(func() {
		tfs, cleanup, err = sysmock.TestFS(map[string]any{
			"/some/root/file1":        []byte("file1"),
			"/some/root/file2":        []byte("file2"),
			"/some/root/subdir/file3": []byte("file3"),
		})
		Expect(err).NotTo(HaveOccurred())

		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("creates a CPIO file with the given root and it extracts it back somewhere else", func() {
		Expect(vfs.MkdirAll(tfs, "/output", vfs.DirPerm)).To(Succeed())

		Expect(cpio.CreateCPIO(context.Background(), s, "/some/root", "/output/cpiofile")).To(Succeed())
		Expect(cpio.ExtractCPIO(context.Background(), s, "/output/cpiofile", "/extraction")).To(Succeed())

		data, err := tfs.ReadFile("/extraction/subdir/file3")
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal([]byte("file3")))
	})
})
