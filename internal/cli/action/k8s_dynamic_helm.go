/*
Copyright © 2025-2026 SUSE LLC
SPDX-License-Identifier: Apache-2.0
*/

package action

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/dynamicdata"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type runtimeHelmResult struct {
	Applied     bool
	KnownCharts []string
}

type runtimeHelmChart struct {
	APIVersion string         `yaml:"apiVersion,omitempty"`
	Kind       string         `yaml:"kind,omitempty"`
	Metadata   map[string]any `yaml:"metadata,omitempty"`
	Spec       map[string]any `yaml:"spec,omitempty"`
}

type runtimeHelmChartFile struct {
	Path     string
	Resource runtimeHelmChart
}

func applyRuntimeHelmOverrides(s *sys.System, k8sConfigDir string, userData *dynamicdata.Data) (runtimeHelmResult, error) {
	overrides, ok := runtimeHelmOverrides(userData)
	charts, err := readRuntimeHelmCharts(s, filepath.Join(k8sConfigDir, filepath.Base(image.HelmPath())))
	if err != nil {
		return runtimeHelmResult{}, err
	}

	result := runtimeHelmResult{KnownCharts: sortedChartNames(charts)}
	if !ok || len(overrides) == 0 {
		result.Applied = true
		return result, nil
	}

	validatedOverrides, overrideNames, err := validateRuntimeHelmOverrides(overrides, charts)
	if err != nil {
		return result, err
	}

	for _, name := range overrideNames {
		chart := charts[name]
		override := validatedOverrides[name]
		existingValues := map[string]any{}
		if rawValues, ok := chart.Resource.Spec["valuesContent"].(string); ok && rawValues != "" {
			decoder := yaml.NewDecoder(bytes.NewBufferString(rawValues))
			decoder.KnownFields(false)
			if err := decoder.Decode(&existingValues); err != nil {
				return result, fmt.Errorf("unmarshaling existing Helm values for chart %s: %w", name, err)
			}
		}

		merged := mergeRuntimeHelmMaps(existingValues, override)
		values, err := yaml.Marshal(merged)
		if err != nil {
			return result, fmt.Errorf("marshaling runtime Helm values for chart %s: %w", name, err)
		}

		if chart.Resource.Spec == nil {
			chart.Resource.Spec = map[string]any{}
		}
		chart.Resource.Spec["valuesContent"] = string(values)
		data, err := yaml.Marshal(chart.Resource)
		if err != nil {
			return result, fmt.Errorf("marshaling HelmChart resource %s: %w", name, err)
		}

		if err := s.FS().WriteFile(chart.Path, data, 0o644); err != nil {
			return result, fmt.Errorf("writing HelmChart resource %s: %w", name, err)
		}
	}

	result.Applied = true
	return result, nil
}

func runtimeHelmOverrides(userData *dynamicdata.Data) (map[string]any, bool) {
	if userData == nil || userData.Values == nil {
		return nil, false
	}

	helmData, ok := userData.Values["helm"].(map[string]any)
	if !ok {
		return nil, false
	}

	valuesData, ok := helmData["values"].(map[string]any)
	if !ok {
		return nil, false
	}

	return valuesData, true
}

func validateRuntimeHelmOverrides(overrides map[string]any, charts map[string]runtimeHelmChartFile) (map[string]map[string]any, []string, error) {
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)

	validated := make(map[string]map[string]any, len(overrides))
	for _, name := range names {
		rawOverride := overrides[name]
		if _, ok := charts[name]; !ok {
			return nil, names, fmt.Errorf("unknown runtime Helm value override: %s", name)
		}

		override, ok := rawOverride.(map[string]any)
		if !ok || override == nil {
			return nil, names, fmt.Errorf("runtime Helm value override for chart %s must be a map", name)
		}
		validated[name] = override
	}

	return validated, names, nil
}

func readRuntimeHelmCharts(s *sys.System, helmDir string) (map[string]runtimeHelmChartFile, error) {
	charts := map[string]runtimeHelmChartFile{}

	isDir, err := vfs.IsDir(s.FS(), helmDir)
	if err != nil || !isDir {
		return charts, nil
	}

	entries, err := s.FS().ReadDir(helmDir)
	if err != nil {
		return nil, fmt.Errorf("reading HelmChart directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(helmDir, entry.Name())
		data, err := s.FS().ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading HelmChart resource %s: %w", entry.Name(), err)
		}

		var chart runtimeHelmChart
		if err := yaml.Unmarshal(data, &chart); err != nil {
			return nil, fmt.Errorf("unmarshaling HelmChart resource %s: %w", entry.Name(), err)
		}
		name, _ := chart.Metadata["name"].(string)
		if chart.Kind != "HelmChart" || name == "" {
			continue
		}

		charts[name] = runtimeHelmChartFile{Path: path, Resource: chart}
	}

	return charts, nil
}

func sortedChartNames(charts map[string]runtimeHelmChartFile) []string {
	names := make([]string, 0, len(charts))
	for name := range charts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mergeRuntimeHelmMaps(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}

	for k, v := range override {
		innerOverride, overrideIsMap := v.(map[string]any)
		innerBase, baseIsMap := out[k].(map[string]any)
		if overrideIsMap && baseIsMap {
			out[k] = mergeRuntimeHelmMaps(innerBase, innerOverride)
			continue
		}
		out[k] = v
	}

	return out
}
