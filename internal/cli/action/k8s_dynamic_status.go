/*
Copyright © 2025-2026 SUSE LLC
SPDX-License-Identifier: Apache-2.0
*/

package action

import (
	"fmt"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	k8sDynamicStatusDir  = "/var/lib/elemental/k8s-dynamic"
	k8sDynamicStatusPath = "/var/lib/elemental/k8s-dynamic/status.yaml"
)

type k8sDynamicStatus struct {
	UserData  userDataStatus  `yaml:"userdata"`
	SSH       applyStatus     `yaml:"ssh"`
	RKE2      applyStatus     `yaml:"rke2"`
	Helm      helmStatus      `yaml:"helm"`
	Resources resourcesStatus `yaml:"resources"`
}

type userDataStatus struct {
	Source  string `yaml:"source,omitempty"`
	Fetched bool   `yaml:"fetched"`
	Error   string `yaml:"error,omitempty"`
}

type applyStatus struct {
	Applied bool   `yaml:"applied"`
	Error   string `yaml:"error,omitempty"`
}

type helmStatus struct {
	OverridesApplied bool     `yaml:"overridesApplied"`
	Error            string   `yaml:"error,omitempty"`
	KnownCharts      []string `yaml:"knownCharts,omitempty"`
}

type resourcesStatus struct {
	DeployResources bool   `yaml:"deployResources"`
	Error           string `yaml:"error,omitempty"`
}

func writeK8sDynamicStatus(s *sys.System, status k8sDynamicStatus) error {
	if err := vfs.MkdirAll(s.FS(), k8sDynamicStatusDir, vfs.DirPerm); err != nil {
		return fmt.Errorf("creating k8s dynamic status directory: %w", err)
	}

	data, err := yaml.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshaling k8s dynamic status: %w", err)
	}

	if err := s.FS().WriteFile(filepath.Clean(k8sDynamicStatusPath), data, 0o644); err != nil {
		return fmt.Errorf("writing k8s dynamic status: %w", err)
	}

	return nil
}
