package dynamicservice_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/dynamicservice"
)

func TestK8sDynamicDefaultsConfigPath(t *testing.T) {
	cfg := dynamicservice.Config{}
	require.False(t, cfg.K8sDynamicEnabled())

	cfg.Services.K8sDynamic.Enabled = true
	cfg.Default()

	require.True(t, cfg.K8sDynamicEnabled())
	require.Equal(t, "/var/lib/elemental/k8s-dynamic/userdata.yaml", cfg.Services.K8sDynamic.Config)
	require.Equal(t, 120, cfg.Services.K8sDynamic.Timeout)
}

func TestParseDynamicServiceDeclaration(t *testing.T) {
	raw := []byte(`
services:
  k8s-dynamic:
    enabled: true
    config: /custom/userdata.yaml
    timeout: 45
`)
	var cfg dynamicservice.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	cfg.Default()

	require.True(t, cfg.K8sDynamicEnabled())
	require.Equal(t, "/custom/userdata.yaml", cfg.Services.K8sDynamic.Config)
	require.Equal(t, 45, cfg.Services.K8sDynamic.Timeout)
}
