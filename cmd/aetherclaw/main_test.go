package main

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal"
)

func TestNewAetherClawCommand(t *testing.T) {
	cmd := NewAetherClawCommand()

	require.NotNil(t, cmd)

	short := fmt.Sprintf("%s aetherclaw - Personal AI Assistant v%s\n\n", internal.Logo, internal.GetVersion())

	assert.Equal(t, "aetherclaw", cmd.Use)
	assert.Equal(t, short, cmd.Short)

	assert.True(t, cmd.HasSubCommands())
	assert.True(t, cmd.HasAvailableSubCommands())

	assert.NotNil(t, cmd.PersistentPreRun)

	assert.Nil(t, cmd.Run)
	assert.Nil(t, cmd.RunE)

	allowedCommands := []string{
		"agent",
		"auth",
		"cron",
		"gateway",
		"migrate",
		"onboard",
		"skills",
		"status",
		"version",
	}

	subcommands := cmd.Commands()
	assert.Len(t, subcommands, len(allowedCommands))

	for _, subcmd := range subcommands {
		found := slices.Contains(allowedCommands, subcmd.Name())
		assert.True(t, found, "unexpected subcommand %q", subcmd.Name())

		assert.False(t, subcmd.Hidden)
	}
}

func TestBannerFlags(t *testing.T) {
	cmd := NewAetherClawCommand()

	bannerFlag := cmd.PersistentFlags().Lookup("banner")
	require.NotNil(t, bannerFlag)
	assert.Equal(t, "false", bannerFlag.DefValue)

	noBannerFlag := cmd.PersistentFlags().Lookup("no-banner")
	require.NotNil(t, noBannerFlag)
	assert.Equal(t, "false", noBannerFlag.DefValue)
}
