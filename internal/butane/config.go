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

package butane

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	base "github.com/coreos/butane/base/v0_6"
	"github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	ignitionv35 "github.com/coreos/ignition/v2/config/v3_5"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

// Config represents a basic butane configuration
type Config struct {
	Version  string        `yaml:"version"`
	Variant  string        `yaml:"variant"`
	Ignition base.Ignition `yaml:"ignition"`
	Passwd   base.Passwd   `yaml:"passwd"`
	Storage  base.Storage  `yaml:"storage"`
	Systemd  base.Systemd  `yaml:"systemd"`
}

// MergeInlineIgnition adds the given inline ignition configuration as a new merge in butane
func (c *Config) MergeInlineIgnition(ignitionConf string) {
	var merge base.Resource

	merge.Inline = &ignitionConf

	c.Ignition.Config.Merge = append(c.Ignition.Config.Merge, merge)
}

// AddSystemdUnit adds an inline unit object in butane configuration
func (c *Config) AddSystemdUnit(name, contents string, enabled bool) {
	var unit base.Unit

	// Only set contents if non-empty (enables service without overriding unit file)
	if contents != "" {
		unit.Contents = &contents
	}
	unit.Enabled = &enabled
	unit.Name = name

	c.Systemd.Units = append(c.Systemd.Units, unit)
}

// WriteIgnitionFile writes an ignition file for the current butane configuration to the given path
func WriteIgnitionFile(s *sys.System, butane any, ignitionFile string) error {
	ignitionBytes, err := TranslateBytes(s, butane)
	if err != nil {
		return err
	}

	dir := filepath.Dir(ignitionFile)
	if dir != "." {
		err = vfs.MkdirAll(s.FS(), dir, vfs.DirPerm)
		if err != nil {
			return fmt.Errorf("could not create ignition file folder: %w", err)
		}
	}

	err = s.FS().WriteFile(ignitionFile, ignitionBytes, vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("failed writing ignition file: %w", err)
	}
	return nil
}

// WriteMergedIgnitionFile translates childButane, merges it over parentIgnition,
// and writes the flattened Ignition config.
func WriteMergedIgnitionFile(s *sys.System, parentIgnition []byte, childButane any, ignitionFile string) error {
	childIgnition, err := TranslateBytes(s, childButane)
	if err != nil {
		return err
	}
	ignitionBytes, err := MergeIgnition(parentIgnition, childIgnition)
	if err != nil {
		return err
	}
	dir := filepath.Dir(ignitionFile)
	if dir != "." {
		err = vfs.MkdirAll(s.FS(), dir, vfs.DirPerm)
		if err != nil {
			return fmt.Errorf("could not create ignition file folder: %w", err)
		}
	}
	err = s.FS().WriteFile(ignitionFile, ignitionBytes, vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("failed writing ignition file: %w", err)
	}
	return nil
}

// MergeIgnition merges childIgnition over parentIgnition using Ignition's typed
// merge semantics and returns a flattened Ignition config.
func MergeIgnition(parentIgnition, childIgnition []byte) ([]byte, error) {
	parent, report, err := ignitionv35.Parse(parentIgnition)
	if err != nil {
		return nil, fmt.Errorf("failed parsing parent Ignition config: %w\nReport: %v", err, report)
	}
	if report.IsFatal() {
		return nil, fmt.Errorf("failed parsing parent Ignition config: %v", report)
	}
	child, report, err := ignitionv35.Parse(childIgnition)
	if err != nil {
		return nil, fmt.Errorf("failed parsing child Ignition config: %w\nReport: %v", err, report)
	}
	if report.IsFatal() {
		return nil, fmt.Errorf("failed parsing child Ignition config: %v", report)
	}
	merged := ignitionv35.Merge(parent, child)
	ignitionBytes, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed marshaling merged Ignition config: %w", err)
	}
	return append(ignitionBytes, '\n'), nil
}

// TranslateBytes translates the given butane configuration to ignition bytes
func TranslateBytes(s *sys.System, butane any) ([]byte, error) {
	butaneBytes, err := yaml.Marshal(butane)
	if err != nil {
		return nil, fmt.Errorf("failed marshalling butane configuration: %w", err)
	}

	ignitionBytes, report, err := config.TranslateBytes(butaneBytes, common.TranslateBytesOptions{Pretty: true})
	if err != nil {
		return nil, fmt.Errorf("failed translating Butane config: %w\nReport: %v", err, report)
	}
	if len(report.Entries) > 0 {
		s.Logger().Warn("translating Butane to Ignition reported non-fatal entries: %v", report)
	}
	s.Logger().Debug("Butane configuration translated:\n--- Generated Ignition Config ---\n%s", string(ignitionBytes))
	return ignitionBytes, nil
}
