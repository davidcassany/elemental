package cmd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	cmdpkg "github.com/suse/elemental/v3/internal/cli/cmd"
)

func TestCustomizeModeMergeAccepted(t *testing.T) {
	cmdpkg.CustomizeArgs = cmdpkg.CustomizeFlags{Mode: "merge"}
	t.Cleanup(func() { cmdpkg.CustomizeArgs = cmdpkg.CustomizeFlags{} })

	cmd := cmdpkg.NewCustomizeCommand("elemental3", func(context.Context, *cli.Command) error {
		return nil
	})
	_, err := cmd.Before(context.Background(), cmd)

	require.NoError(t, err)
}

func TestCustomizeModeUnknownRejected(t *testing.T) {
	cmdpkg.CustomizeArgs = cmdpkg.CustomizeFlags{Mode: "invalid"}
	t.Cleanup(func() { cmdpkg.CustomizeArgs = cmdpkg.CustomizeFlags{} })

	cmd := cmdpkg.NewCustomizeCommand("elemental3", func(context.Context, *cli.Command) error {
		return nil
	})
	_, err := cmd.Before(context.Background(), cmd)

	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported --mode option")
}
