//go:build integration

package mergemode

import (
	"os"
	"testing"
)

type proxmoxEnv struct {
	Host        string
	Node        string
	Storage     string
	VMID        string
	SourceImage string
}

func requireProxmoxEnv(t *testing.T) proxmoxEnv {
	t.Helper()
	env := proxmoxEnv{
		Host:        os.Getenv("ELEMENTAL_PROXMOX_HOST"),
		Node:        os.Getenv("ELEMENTAL_PROXMOX_NODE"),
		Storage:     os.Getenv("ELEMENTAL_PROXMOX_STORAGE"),
		VMID:        os.Getenv("ELEMENTAL_PROXMOX_VMID"),
		SourceImage: os.Getenv("ELEMENTAL_PROXMOX_SOURCE_IMAGE"),
	}
	if env.Host == "" || env.Node == "" || env.Storage == "" || env.VMID == "" || env.SourceImage == "" {
		t.Skip("set ELEMENTAL_PROXMOX_HOST, ELEMENTAL_PROXMOX_NODE, ELEMENTAL_PROXMOX_STORAGE, ELEMENTAL_PROXMOX_VMID, and ELEMENTAL_PROXMOX_SOURCE_IMAGE to run Proxmox merge-mode integration tests")
	}
	return env
}

func TestIntegrationEnvironmentGate(t *testing.T) {
	requireProxmoxEnv(t)
}
