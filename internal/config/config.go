package config

// Config holds TUI startup configuration from CLI flags and env.
type Config struct {
	URL      string
	Token    string
	Password string
	Session  string
	Version  string
}
