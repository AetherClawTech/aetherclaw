// AetherClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 AetherClaw contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/agent"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/auth"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/cron"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/gateway"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/migrate"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/onboard"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/skills"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/status"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/version"
)

func NewPicoclawCommand() *cobra.Command {
	short := fmt.Sprintf("%s aetherclaw - Personal AI Assistant v%s\n\n", internal.Logo, internal.GetVersion())

	cmd := &cobra.Command{
		Use:     "aetherclaw",
		Short:   short,
		Example: "aetherclaw list",
	}

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		auth.NewAuthCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		migrate.NewMigrateCommand(),
		skills.NewSkillsCommand(),
		version.NewVersionCommand(),
	)

	return cmd
}

func main() {
	cmd := NewPicoclawCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
