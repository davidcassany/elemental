/*
Copyright © 2025-2026 SUSE LLC
SPDX-License-Identifier: Apache-2.0
*/

package action

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/userdata"
)

var _ = Describe("writeHostnameFromUserData", Label("k8s-dynamic", "hostname"), func() {
	var (
		system  *sys.System
		runner  *sysmock.Runner
		cleanup func()
	)

	BeforeEach(func() {
		fs, c, err := sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		cleanup = c
		runner = sysmock.NewRunner()

		system, err = sys.NewSystem(
			sys.WithFS(fs),
			sys.WithLogger(log.New()),
			sys.WithRunner(runner),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("writes hostname to /etc/hostname and applies it via hostnamectl", func() {
		ud := &userdata.UserData{
			Data:     map[string]any{"hostname": "node1.example.com"},
			Provider: "test",
		}

		err := writeHostnameFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())

		content, err := system.FS().ReadFile("/etc/hostname")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("node1.example.com\n"))

		Expect(runner.IncludesCmds([][]string{
			{"hostnamectl", "set-hostname", "node1.example.com"},
		})).To(Succeed())
	})

	It("does nothing when hostname is not set", func() {
		ud := &userdata.UserData{
			Data:     map[string]any{"rke2": map[string]any{"type": "server"}},
			Provider: "test",
		}

		err := writeHostnameFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())
	})

	It("does nothing when hostname is empty", func() {
		ud := &userdata.UserData{
			Data:     map[string]any{"hostname": ""},
			Provider: "test",
		}

		err := writeHostnameFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())
	})

	It("does nothing when user data is nil", func() {
		err := writeHostnameFromUserData(system, nil)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("writeSSHKeysFromUserData", Label("k8s-dynamic", "ssh"), func() {
	var (
		system  *sys.System
		cleanup func()
	)

	BeforeEach(func() {
		fs, c, err := sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		cleanup = c

		system, err = sys.NewSystem(
			sys.WithFS(fs),
			sys.WithLogger(log.New()),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("writes SSH keys to /root/.ssh/authorized_keys for root user", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"users": []any{
					map[string]any{
						"name": "root",
						"ssh_authorized_keys": []any{
							"ssh-ed25519 AAAA-test-key user@host",
						},
					},
				},
			},
			Provider: "test",
		}

		err := writeSSHKeysFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())

		content, err := system.FS().ReadFile("/root/.ssh/authorized_keys")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("ssh-ed25519 AAAA-test-key user@host"))
	})

	It("appends to existing authorized_keys", func() {
		// Pre-existing key (e.g., set by Ignition)
		Expect(vfs.MkdirAll(system.FS(), "/root/.ssh", 0o700)).To(Succeed())
		Expect(system.FS().WriteFile("/root/.ssh/authorized_keys", []byte("ssh-rsa existing-key\n"), 0o600)).To(Succeed())

		ud := &userdata.UserData{
			Data: map[string]any{
				"users": []any{
					map[string]any{
						"name": "root",
						"ssh_authorized_keys": []any{
							"ssh-ed25519 new-key",
						},
					},
				},
			},
			Provider: "test",
		}

		err := writeSSHKeysFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())

		content, err := system.FS().ReadFile("/root/.ssh/authorized_keys")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("ssh-rsa existing-key"))
		Expect(string(content)).To(ContainSubstring("ssh-ed25519 new-key"))
	})

	It("handles multiple users with multiple keys", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"users": []any{
					map[string]any{
						"name": "root",
						"ssh_authorized_keys": []any{
							"ssh-ed25519 key1",
							"ssh-ed25519 key2",
						},
					},
				},
			},
			Provider: "test",
		}

		err := writeSSHKeysFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())

		content, err := system.FS().ReadFile("/root/.ssh/authorized_keys")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("ssh-ed25519 key1"))
		Expect(string(content)).To(ContainSubstring("ssh-ed25519 key2"))
	})

	It("does nothing when no users section exists", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"rke2": map[string]any{"type": "server"},
			},
			Provider: "test",
		}

		err := writeSSHKeysFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())
		// No file should be created
		_, err = system.FS().ReadFile("/root/.ssh/authorized_keys")
		Expect(err).To(HaveOccurred())
	})

	It("does nothing when user data is nil", func() {
		err := writeSSHKeysFromUserData(system, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("skips users without ssh_authorized_keys", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"users": []any{
					map[string]any{
						"name": "root",
					},
				},
			},
			Provider: "test",
		}

		err := writeSSHKeysFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("writeK8sDynamicDeployScript", Label("k8s-dynamic", "deploy-script"), func() {
	var (
		system  *sys.System
		cleanup func()
	)

	BeforeEach(func() {
		fs, c, err := sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		cleanup = c

		system, err = sys.NewSystem(
			sys.WithFS(fs),
			sys.WithLogger(log.New()),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("installs embedded RKE2 artifacts before enabling the node service", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"rke2": map[string]any{
					"type":  "server",
					"init":  true,
					"token": "test-token",
				},
			},
			Provider: "test",
		}

		err := writeK8sDynamicDeployScript(system, "/var/lib/elemental/kubernetes", ud)
		Expect(err).NotTo(HaveOccurred())

		content, err := system.FS().ReadFile("/var/lib/elemental/kubernetes/k8s_conf_deploy.sh")
		Expect(err).NotTo(HaveOccurred())
		script := string(content)

		Expect(script).To(ContainSubstring("export INSTALL_RKE2_ARTIFACT_PATH=\"/opt/k8s/install\""))
		Expect(script).To(ContainSubstring("sh \"/opt/k8s/install/install.sh\""))
		Expect(script).To(ContainSubstring("NODETYPE=\"server\""))
		Expect(script).To(ContainSubstring("systemctl enable --now rke2-${NODETYPE}.service"))
		Expect(script).To(MatchRegexp(`(?s)sh "/opt/k8s/install/install\.sh".*systemctl enable --now rke2-\$\{NODETYPE\}\.service`))
	})
})

var _ = Describe("k8s dynamic status", Label("k8s-dynamic", "status"), func() {
	var (
		system  *sys.System
		cleanup func()
	)

	BeforeEach(func() {
		fs, c, err := sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		cleanup = c

		system, err = sys.NewSystem(
			sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("writes sanitized persistent status without raw Helm values", func() {
		status := k8sDynamicStatus{
			UserData: userDataStatus{Provider: "aws", Fetched: true},
			SSH:      applyStatus{Applied: true},
			RKE2:     applyStatus{Applied: true},
			Helm: helmStatus{
				OverridesApplied: false,
				Error:            "unknown runtime Helm value override: certmanager",
				KnownCharts:      []string{"cert-manager", "rancher"},
			},
			Resources: resourcesStatus{DeployResources: false},
		}

		Expect(writeK8sDynamicStatus(system, status)).To(Succeed())

		content, err := system.FS().ReadFile(k8sDynamicStatusPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("provider: aws"))
		Expect(string(content)).To(ContainSubstring("overridesApplied: false"))
		Expect(string(content)).To(ContainSubstring("knownCharts:"))
		Expect(string(content)).To(ContainSubstring("deployResources: false"))
		Expect(string(content)).NotTo(ContainSubstring("super-secret"))
		Expect(string(content)).NotTo(ContainSubstring("valuesContent"))
	})
})

var _ = Describe("applyRuntimeHelmOverrides", Label("k8s-dynamic", "helm"), func() {
	var (
		system  *sys.System
		cleanup func()
	)

	BeforeEach(func() {
		fs, c, err := sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		cleanup = c

		system, err = sys.NewSystem(
			sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(vfs.MkdirAll(system.FS(), "/var/lib/elemental/kubernetes/helm", vfs.DirPerm)).To(Succeed())
		Expect(system.FS().WriteFile("/var/lib/elemental/kubernetes/helm/rancher.yaml", []byte(`apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
  name: rancher
  namespace: kube-system
spec:
  chart: rancher
  version: 2.10.0
  repo: https://charts.rancher.io
  targetNamespace: cattle-system
  valuesContent: |
    hostname: old.example.com
    replicas: 1
    ingress:
      tls:
        source: secret
        enabled: false
    extraArgs:
    - a
`), 0o644)).To(Succeed())
	})

	AfterEach(func() {
		cleanup()
	})

	It("recursively merges runtime values into existing HelmChart valuesContent", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"helm": map[string]any{
					"values": map[string]any{
						"rancher": map[string]any{
							"hostname": "new.example.com",
							"ingress": map[string]any{
								"tls": map[string]any{
									"enabled": true,
								},
							},
							"extraArgs": []any{"b", "c"},
						},
					},
				},
			},
			Provider: "test",
		}

		result, err := applyRuntimeHelmOverrides(system, "/var/lib/elemental/kubernetes", ud)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Applied).To(BeTrue())
		Expect(result.KnownCharts).To(Equal([]string{"rancher"}))

		content, err := system.FS().ReadFile("/var/lib/elemental/kubernetes/helm/rancher.yaml")
		Expect(err).NotTo(HaveOccurred())
		chart := string(content)
		Expect(chart).To(ContainSubstring("hostname: new.example.com"))
		Expect(chart).To(ContainSubstring("replicas: 1"))
		Expect(chart).To(ContainSubstring("source: secret"))
		Expect(chart).To(ContainSubstring("enabled: true"))
		Expect(chart).To(ContainSubstring("- b"))
		Expect(chart).To(ContainSubstring("- c"))
		Expect(chart).NotTo(ContainSubstring("- a"))
	})

	It("reports unknown chart names without writing raw override values", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"helm": map[string]any{
					"values": map[string]any{
						"certmanager": map[string]any{
							"password": "super-secret",
						},
					},
				},
			},
			Provider: "test",
		}

		result, err := applyRuntimeHelmOverrides(system, "/var/lib/elemental/kubernetes", ud)
		Expect(err).To(MatchError("unknown runtime Helm value override: certmanager"))
		Expect(result.Applied).To(BeFalse())
		Expect(result.KnownCharts).To(Equal([]string{"rancher"}))
		Expect(err.Error()).NotTo(ContainSubstring("super-secret"))
	})

	It("rejects a chart override root that is not a map", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"helm": map[string]any{
					"values": map[string]any{
						"rancher": "new.example.com",
					},
				},
			},
			Provider: "test",
		}

		result, err := applyRuntimeHelmOverrides(system, "/var/lib/elemental/kubernetes", ud)
		Expect(err).To(MatchError("runtime Helm value override for chart rancher must be a map"))
		Expect(result.Applied).To(BeFalse())
		Expect(result.KnownCharts).To(Equal([]string{"rancher"}))
	})

	It("allows SSH setup to complete before returning a recoverable Helm error", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"users": []any{
					map[string]any{
						"name": "root",
						"ssh_authorized_keys": []any{
							"ssh-ed25519 recovery-key",
						},
					},
				},
				"helm": map[string]any{
					"values": map[string]any{
						"missing": map[string]any{
							"token": "super-secret",
						},
					},
				},
			},
			Provider: "test",
		}

		status, err := applyDynamicConfigurationFromUserData(system, "/var/lib/elemental/kubernetes", ud)
		Expect(err).To(MatchError("unknown runtime Helm value override: missing"))

		keys, keyErr := system.FS().ReadFile("/root/.ssh/authorized_keys")
		Expect(keyErr).NotTo(HaveOccurred())
		Expect(string(keys)).To(ContainSubstring("ssh-ed25519 recovery-key"))
		Expect(status.SSH.Applied).To(BeTrue())
		Expect(status.Helm.OverridesApplied).To(BeFalse())
		Expect(status.Helm.Error).To(Equal("unknown runtime Helm value override: missing"))
		Expect(status.Helm.Error).NotTo(ContainSubstring("super-secret"))

		content, readErr := system.FS().ReadFile(k8sDynamicStatusPath)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("unknown runtime Helm value override: missing"))
		Expect(string(content)).NotTo(ContainSubstring("super-secret"))
	})

	It("does not allow runtime values to change chart identity fields", func() {
		ud := &userdata.UserData{
			Data: map[string]any{
				"helm": map[string]any{
					"values": map[string]any{
						"rancher": map[string]any{
							"chart":   "different-chart",
							"repo":    "https://evil.example.com",
							"version": "0.0.1",
						},
					},
				},
			},
			Provider: "test",
		}

		_, err := applyRuntimeHelmOverrides(system, "/var/lib/elemental/kubernetes", ud)
		Expect(err).NotTo(HaveOccurred())

		content, err := system.FS().ReadFile("/var/lib/elemental/kubernetes/helm/rancher.yaml")
		Expect(err).NotTo(HaveOccurred())
		chart := string(content)
		Expect(chart).To(ContainSubstring("chart: rancher"))
		Expect(chart).To(ContainSubstring("version: 2.10.0"))
		Expect(chart).To(ContainSubstring("repo: https://charts.rancher.io"))
		Expect(chart).To(ContainSubstring("valuesContent:"))
		Expect(chart).To(ContainSubstring("chart: different-chart"))
		Expect(chart).To(ContainSubstring("repo: https://evil.example.com"))
		Expect(chart).To(ContainSubstring("version: 0.0.1"))
	})
})

var _ = Describe("writeResourceDeployMarkerFromUserData", Label("k8s-dynamic", "resources"), func() {
	var (
		system  *sys.System
		cleanup func()
	)

	BeforeEach(func() {
		fs, c, err := sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		cleanup = c

		system, err = sys.NewSystem(
			sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("defaults omitted deployResources to true and writes marker", func() {
		status, err := writeResourceDeployMarkerFromUserData(system, &userdata.UserData{Data: map[string]any{}})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.DeployResources).To(BeTrue())
		exists, err := vfs.Exists(system.FS(), k8sDynamicDeployResourcesMarkerPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("writes marker when deployResources is true", func() {
		ud := &userdata.UserData{Data: map[string]any{
			"elemental": map[string]any{
				"kubernetes": map[string]any{
					"deployResources": true,
				},
			},
		}}

		status, err := writeResourceDeployMarkerFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.DeployResources).To(BeTrue())
		exists, err := vfs.Exists(system.FS(), k8sDynamicDeployResourcesMarkerPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("removes marker when deployResources is false even for an init server", func() {
		ud := &userdata.UserData{Data: map[string]any{
			"rke2": map[string]any{
				"type": "server",
				"init": true,
			},
			"elemental": map[string]any{
				"kubernetes": map[string]any{
					"deployResources": false,
				},
			},
		}}

		status, err := writeResourceDeployMarkerFromUserData(system, ud)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.DeployResources).To(BeFalse())
		exists, err := vfs.Exists(system.FS(), k8sDynamicDeployResourcesMarkerPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse())
	})
})
