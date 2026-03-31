package main

import (
	"fmt"
	"os"

	"github.com/evancohen/openclaw-tui/internal/config"
	"github.com/evancohen/openclaw-tui/internal/model"
	"github.com/spf13/cobra"

	tea "charm.land/bubbletea/v2"
)

var version = "0.1.0"

func main() {
	var cfg config.Config

	rootCmd := &cobra.Command{
		Use:   "openclaw-tui",
		Short: "A Bubble Tea TUI for OpenClaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve token from flag or env
			if cfg.Token == "" {
				cfg.Token = os.Getenv("OPENCLAW_GATEWAY_TOKEN")
			}
			if cfg.Password == "" {
				cfg.Password = os.Getenv("OPENCLAW_GATEWAY_PASSWORD")
			}

			cfg.Version = version

			p := tea.NewProgram(
				model.New(cfg),
			)

			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}

	rootCmd.Flags().StringVar(&cfg.URL, "url", "ws://127.0.0.1:18789", "Gateway WebSocket URL")
	rootCmd.Flags().StringVar(&cfg.Token, "token", "", "Gateway auth token")
	rootCmd.Flags().StringVar(&cfg.Password, "password", "", "Gateway auth password")
	rootCmd.Flags().StringVar(&cfg.Session, "session", "", "Initial session key")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
