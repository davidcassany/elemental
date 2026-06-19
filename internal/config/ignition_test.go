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

package config

import (
	"bytes"
	"path/filepath"

	ignitionv35 "github.com/coreos/ignition/v2/config/v3_5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v0 "github.com/suse/elemental/v3/internal/config/v0"
	"github.com/suse/elemental/v3/internal/dynamicservice"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Ignition configuration", func() {
	var output = Output{
		RootPath: "/_out",
	}

	var system *sys.System
	var fs vfs.FS
	var cleanup func()
	var err error
	var m *Manager
	var buffer *bytes.Buffer

	BeforeEach(func() {
		buffer = &bytes.Buffer{}
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/etc/kubernetes/config/server.yaml":     "",
			"/etc/kubernetes/config/registries.yaml": "key: 'value'\n",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(vfs.MkdirAll(fs, output.RootPath, vfs.DirPerm)).To(Succeed())

		system, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())

		m = NewManager(system, nil)
	})

	AfterEach(func() {
		cleanup()
	})

	It("Does no Ignition configuration if data is not provided", func() {
		conf := &image.Configuration{}

		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, output, "", "", nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translates given ButaneConfig to an Ignition file as an embedded merge", func() {
		var butaneConf map[string]any

		butaneConfigString := `
version: 1.6.0
variant: fcos
passwd:
  users:
  - name: pipo
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`

		Expect(v0.ParseAny([]byte(butaneConfigString), &butaneConf)).To(Succeed())

		conf := &image.Configuration{
			ButaneConfig: butaneConf,
		}

		Expect(err).NotTo(HaveOccurred())

		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, output, "", "", nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("merge"))
	})

	It("Writes merge marker out of band from embedded Ignition", func() {
		var butaneConf map[string]any
		butaneConfigString := `
version: 1.6.0
variant: fcos
storage:
  files:
    - path: /var/lib/elemental/base-ignition-marker
      mode: 0644
      contents:
        inline: base-ok
`
		Expect(v0.ParseAny([]byte(butaneConfigString), &butaneConf)).To(Succeed())
		conf := &image.Configuration{
			ButaneConfig: butaneConf,
		}
		mergeOutput := Output{
			RootPath: output.RootPath,
			Mode:     OutputModeMerge,
		}

		Expect(m.configureIgnition(conf, mergeOutput, "", "", nil)).To(Succeed())

		markerPath := filepath.Join(mergeOutput.FirstbootConfigDir(), "ignition", "elemental-merge")
		exists, err := vfs.Exists(fs, markerPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())

		ignitionFile := filepath.Join(mergeOutput.FirstbootConfigDir(), image.IgnitionFilePath())
		exists, err = vfs.Exists(fs, ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("Flattens Butane config into merge-mode Ignition so base.d applies it", func() {
		var butaneConf map[string]any
		butaneConfigString := `
version: 1.6.0
variant: fcos
systemd:
  units:
  - name: sshd.service
    enabled: true
passwd:
  users:
  - name: root
    password_hash: "$6$dkiCjuXvS8brdFUA$w1b4wSV.0wQ7BmZ7l/Be6fhqlk8CMEE8NQkhtaXIPjMTFw90JNYfI1lBhSoUILhmqupcmOp681FHIdvIZdbc90"
    ssh_authorized_keys:
    - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGXOuyGFN7mjSU2xHiq7tg1oEuicftTDPl99vqnJV8ka rnaidoo@stackstate.com
`
		Expect(v0.ParseAny([]byte(butaneConfigString), &butaneConf)).To(Succeed())
		conf := &image.Configuration{
			ButaneConfig: butaneConf,
			DynamicServices: dynamicservice.Config{
				Services: dynamicservice.Services{
					K8sDynamic: dynamicservice.Service{
						Enabled: true,
					},
				},
			},
		}
		mergeOutput := Output{
			RootPath: output.RootPath,
			Mode:     OutputModeMerge,
		}

		Expect(m.configureIgnition(conf, mergeOutput, "", "", nil)).To(Succeed())

		ignitionFile := filepath.Join(mergeOutput.FirstbootConfigDir(), image.IgnitionFilePath())
		ignitionBytes, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		ignitionConfig, report, err := ignitionv35.Parse(ignitionBytes)
		Expect(err).NotTo(HaveOccurred(), report.String())
		Expect(report.IsFatal()).To(BeFalse(), report.String())
		Expect(ignitionConfig.Ignition.Config.Merge).To(BeEmpty())
		rootConfigured := false
		for _, user := range ignitionConfig.Passwd.Users {
			if user.Name == "root" && user.PasswordHash != nil && len(user.SSHAuthorizedKeys) > 0 {
				rootConfigured = true
			}
		}
		Expect(rootConfigured).To(BeTrue())
		sshdEnabled := false
		k8sDynamicEnabled := false
		for _, unit := range ignitionConfig.Systemd.Units {
			if unit.Name == "sshd.service" && unit.Enabled != nil && *unit.Enabled {
				sshdEnabled = true
			}
			if unit.Name == k8sDynamicUnitName {
				k8sDynamicEnabled = true
			}
		}
		Expect(sshdEnabled).To(BeTrue())
		Expect(k8sDynamicEnabled).To(BeTrue())
	})

	It("Configures kubernetes via Ignition with the given k8s script", func() {
		// includes registries configuration
		conf := &image.Configuration{
			Kubernetes: kubernetes.Kubernetes{
				Config: kubernetes.Config{
					RegistriesFilePath: "/etc/kubernetes/config/registries.yaml",
				},
			},
		}
		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())

		k8sScript := filepath.Join(output.OverlaysDir(), "path/to/k8s/script.sh")
		k8sConfScript := filepath.Join(output.OverlaysDir(), "path/to/k8s/conf_script.sh")

		Expect(m.configureIgnition(conf, output, k8sScript, k8sConfScript, nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).NotTo(ContainSubstring("merge"))
		Expect(ignition).NotTo(ContainSubstring("/etc/elemental/extensions.yaml"))
		Expect(ignition).To(ContainSubstring("Kubernetes Resources Installer"))
		Expect(ignition).To(ContainSubstring("Kubernetes Installation and Configuration"))
		Expect(ignition).To(ContainSubstring("/var/lib/elemental/kubernetes/registries.yaml"))
	})

	It("Configures dynamic kubernetes resources via deploy marker instead of host condition", func() {
		conf := &image.Configuration{
			DynamicServices: dynamicservice.Config{
				Services: dynamicservice.Services{
					K8sDynamic: dynamicservice.Service{Enabled: true},
				},
			},
		}
		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())

		k8sScript := "/var/lib/elemental/kubernetes/k8s_res_deploy.sh"
		k8sConfScript := "/var/lib/elemental/kubernetes/k8s_conf_deploy.sh"

		Expect(m.configureIgnition(conf, output, k8sScript, k8sConfScript, nil)).To(Succeed())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(ignition)).To(ContainSubstring("Kubernetes Resources Installer"))
		Expect(string(ignition)).To(ContainSubstring("After=k8s-config-installer.service\\nConditionPathExists=/run/elemental/k8s-dynamic-deploy-resources"))
		Expect(string(ignition)).To(ContainSubstring("ConditionPathExists=/run/elemental/k8s-dynamic-deploy-resources"))
		Expect(string(ignition)).NotTo(ContainSubstring("ConditionHost=*"))
	})

	It("Configures file-driven k8s-dynamic service in merge mode", func() {
		conf := &image.Configuration{
			DynamicServices: dynamicservice.Config{
				Services: dynamicservice.Services{
					K8sDynamic: dynamicservice.Service{Enabled: true},
				},
			},
		}
		mergeOutput := Output{
			RootPath: output.RootPath,
			Mode:     OutputModeMerge,
		}
		ignitionFile := filepath.Join(mergeOutput.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, mergeOutput, "", "", nil)).To(Succeed())

		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(ignition)).To(ContainSubstring("ExecStart=/usr/bin/elemental3ctl k8s-dynamic apply --config /var/lib/elemental/k8s-dynamic/userdata.yaml"))
		Expect(string(ignition)).NotTo(ContainSubstring("network-online.target"))
	})

	It("Keeps static kubernetes resources on init host only when user data is disabled", func() {
		conf := &image.Configuration{}
		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())

		k8sScript := "/var/lib/elemental/kubernetes/k8s_res_deploy.sh"
		k8sConfScript := "/var/lib/elemental/kubernetes/k8s_conf_deploy.sh"

		Expect(m.configureIgnition(conf, output, k8sScript, k8sConfScript, nil)).To(Succeed())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(ignition)).To(ContainSubstring("After=k8s-config-installer.service\\nConditionHost=*"))
		Expect(string(ignition)).To(ContainSubstring("ConditionHost=*"))
		Expect(string(ignition)).NotTo(ContainSubstring("ConditionPathExists=/run/elemental/k8s-dynamic-deploy-resources"))
	})

	It("Writes systemd extension via Ignition", func() {
		conf := &image.Configuration{}
		ext := []api.SystemdExtension{{Name: "ext1", Image: "ext1-image"}}
		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, output, "", "", ext)).To(Succeed())

		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())

		Expect(ignition).To(ContainSubstring("/etc/elemental/extensions.yaml"))
		Expect(ignition).To(ContainSubstring("Reload systemd units"))
		Expect(ignition).To(ContainSubstring("Reload kernel modules"))
		Expect(ignition).To(ContainSubstring("Update linker cache"))
		Expect(ignition).NotTo(ContainSubstring("merge"))
		Expect(ignition).NotTo(ContainSubstring("Kubernetes Resources Installer"))
		Expect(ignition).NotTo(ContainSubstring("Kubernetes Config Installer"))
		Expect(ignition).NotTo(ContainSubstring("/var/lib/elemental/kubernetes/registries.yaml"))
	})

	It("Fails to translate a butaneConfig with a wrong version or variant", func() {
		var butane map[string]any

		butaneConfigString := `
version: 0.0.1
variant: unknown
passwd:
  users:
  - name: pipo
    ssh_authorized_keys:
    - key1
`
		k8sScript := filepath.Join(output.OverlaysDir(), "path/to/k8s/script.sh")
		k8sConfScript := filepath.Join(output.OverlaysDir(), "path/to/k8s/conf_script.sh")

		Expect(v0.ParseAny([]byte(butaneConfigString), &butane)).To(Succeed())
		conf := &image.Configuration{
			ButaneConfig: butane,
		}

		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, output, k8sScript, k8sConfScript, nil)).To(MatchError(
			ContainSubstring("No translator exists for variant unknown with version"),
		))
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translate a ButaneConfig with unknown keys by ignoring them and throws warning messages", func() {
		var butane map[string]any

		butaneConfigString := `
version: 1.6.0
variant: fcos
passwd:
  usrs:
  - name: pipo
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
		Expect(v0.ParseAny([]byte(butaneConfigString), &butane)).To(Succeed())
		conf := &image.Configuration{
			ButaneConfig: butane,
		}

		ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())
		Expect(m.configureIgnition(conf, output, "", "", nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("merge"))
		Expect(buffer.String()).To(ContainSubstring("translating Butane to Ignition reported non-fatal entries"))
	})
})
