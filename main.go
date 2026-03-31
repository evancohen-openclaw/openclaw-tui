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
	var (
		flagURL         string
		flagToken       string
		flagPassword    string
		flagSession     string
		flagConfig      string
		flagTheme       string
		flagTLSInsecure bool
	)

	rootCmd := &cobra.Command{
		Use:   "openclaw-tui",
		Short: "A terminal UI for OpenClaw",
		Long:  "openclaw-tui connects to an OpenClaw Gateway and provides a rich terminal chat interface with markdown rendering, streaming, and slash commands.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config file first (env vars overlay)
			cfg, err := config.Load(flagConfig)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			// CLI flags override config file + env
			if cmd.Flags().Changed("url") {
				cfg.URL = flagURL
			}
			if cmd.Flags().Changed("token") {
				cfg.Token = flagToken
			}
			if cmd.Flags().Changed("password") {
				cfg.Password = flagPassword
			}
			if cmd.Flags().Changed("session") {
				cfg.Session = flagSession
			}
			if cmd.Flags().Changed("theme") {
				cfg.Theme = flagTheme
			}
			if cmd.Flags().Changed("tls-insecure") {
				cfg.TLSInsecure = flagTLSInsecure
			}

			cfg.Version = version

			p := tea.NewProgram(model.New(cfg))

			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}

	rootCmd.Flags().StringVar(&flagURL, "url", "", "Gateway WebSocket URL (default ws://127.0.0.1:18789)")
	rootCmd.Flags().StringVar(&flagToken, "token", "", "Gateway auth token")
	rootCmd.Flags().StringVar(&flagPassword, "password", "", "Gateway auth password")
	rootCmd.Flags().StringVar(&flagSession, "session", "", "Initial session key")
	rootCmd.Flags().StringVar(&flagConfig, "config", "", "Config file path (default ~/.config/openclaw-tui/config.json)")
	rootCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme: dark or light")
	rootCmd.Flags().BoolVar(&flagTLSInsecure, "tls-insecure", false, "Skip TLS certificate verification (for self-signed certs)")

	// Version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("openclaw-tui %s\n", version)
		},
	})

	// Init command — creates a config file
	rootCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create a config file with current settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := flagConfig
			if path == "" {
				path = config.DefaultConfigPath()
			}

			// Check if file already exists
			if _, err := os.Stat(path); err == nil {
				fmt.Fprintf(os.Stderr, "Config already exists: %s\n", path)
				fmt.Fprintf(os.Stderr, "Edit it directly or delete it first.\n")
				return nil
			}

			cfg := config.Config{
				URL: "ws://127.0.0.1:18789",
			}
			if err := cfg.Save(path); err != nil {
				return err
			}
			fmt.Printf("Created config: %s\n", path)
			return nil
		},
	})

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
