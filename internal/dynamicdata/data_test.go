package dynamicdata_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/suse/elemental/v3/internal/dynamicdata"
)

func TestParseDynamicNodeUserData(t *testing.T) {
	raw := []byte(`
hostname: node1.example.com
rke2:
  type: server
  init: true
  token: test-token
elemental:
  kubernetes:
    deployResources: true
helm:
  values:
    rancher:
      hostname: rancher.example.com
`)

	data, err := dynamicdata.Parse(raw, "file")

	require.NoError(t, err)
	require.Equal(t, "file", data.Source)
	require.Equal(t, "node1.example.com", data.String("hostname"))
	require.Equal(t, "server", data.Map("rke2")["type"])
}

func TestParseRejectsInvalidYAML(t *testing.T) {
	_, err := dynamicdata.Parse([]byte("rke2: ["), "file")

	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing Dynamic Node User Data")
}
