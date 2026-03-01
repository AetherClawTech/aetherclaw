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
	"golang.org/x/term"

	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/agent"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/auth"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/brand"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/cron"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/gateway"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/migrate"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/onboard"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/skills"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/status"
	"github.com/AetherClawTech/aetherclaw/cmd/aetherclaw/internal/version"
)

func NewAetherClawCommand() *cobra.Command {
	var showBanner bool
	var noBanner bool

	short := fmt.Sprintf("%s aetherclaw - Personal AI Assistant v%s\n\n", internal.Logo, internal.GetVersion())

	cmd := &cobra.Command{
		Use:     "aetherclaw",
		Short:   short,
		Example: "aetherclaw agent",
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			if noBanner {
				return
			}
			if showBanner || term.IsTerminal(int(os.Stdout.Fd())) {
				brand.PrintBanner(os.Stdout)
				fmt.Fprintf(os.Stdout, "  v%s\n\n", internal.GetVersion())
			}
		},
	}

	cmd.PersistentFlags().BoolVar(&showBanner, "banner", false, "force display of startup banner")
	cmd.PersistentFlags().BoolVar(&noBanner, "no-banner", false, "suppress startup banner")

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
	cmd := NewAetherClawCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
