# openclaw-tui

A terminal UI for [OpenClaw](https://openclaw.ai) built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- **Streaming** — token-by-token response display
- **Markdown rendering** — assistant messages rendered with [Glamour](https://github.com/charmbracelet/glamour)
- **Slash commands** — `/model`, `/agent`, `/session`, `/think`, `/status`, and more with autocomplete
- **Tool events** — live display of tool calls with status indicators
- **Session management** — switch agents, sessions, and models with overlay pickers
- **Auto-reconnect** — exponential backoff reconnection to the gateway
- **Thinking blocks** — collapsible thinking content display
- **Dark/light themes** — auto-detect or configure

## Install

```bash
go install github.com/evancohen/openclaw-tui@latest
```

Or build from source:

```bash
git clone https://github.com/evancohen/openclaw-tui.git
cd openclaw-tui
go build -o openclaw-tui .
```

## Usage

```bash
# Connect to local gateway (default)
openclaw-tui

# Connect with auth
openclaw-tui --token YOUR_TOKEN

# Connect to remote gateway
openclaw-tui --url ws://your-host:18789 --token YOUR_TOKEN

# Use a specific session
openclaw-tui --session my-session
```

## Configuration

Config file at `~/.config/openclaw-tui/config.json`:

```json
{
  "url": "ws://127.0.0.1:18789",
  "token": "your-gateway-token",
  "theme": "dark"
}
```

Create one with:

```bash
openclaw-tui init
```

**Priority order:** CLI flags > environment variables > config file > defaults.

### Environment Variables

| Variable | Description |
|---|---|
| `OPENCLAW_GATEWAY_URL` | Gateway WebSocket URL |
| `OPENCLAW_GATEWAY_TOKEN` | Auth token |
| `OPENCLAW_GATEWAY_PASSWORD` | Auth password |
| `OPENCLAW_THEME` | `dark` or `light` |

## Slash Commands

| Command | Description |
|---|---|
| `/help` | Show help |
| `/status` | Gateway status |
| `/agent <id>` | Switch agent |
| `/agents` | List agents (overlay picker) |
| `/session <key>` | Switch session |
| `/sessions` | List sessions (overlay picker) |
| `/model <provider/model>` | Set model |
| `/models` | List models (overlay picker) |
| `/think <level>` | Set thinking level |
| `/new` | New session |
| `/reset` | Reset current session |
| `/abort` | Abort active run |
| `/exit` | Exit |

## Keyboard Shortcuts

| Key | Action |
|---|---|
| `Enter` | Send message |
| `Tab` | Accept autocomplete suggestion |
| `↑/↓` | Navigate autocomplete/overlay |
| `Escape` | Dismiss autocomplete/overlay |
| `Ctrl+C` | Clear input / double-tap to exit |
| `Ctrl+D` | Exit immediately |

## License

MIT
