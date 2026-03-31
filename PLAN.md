# OpenClaw TUI — Build Plan

A native Go terminal client for OpenClaw, built with the Charm stack (Bubble Tea, Bubbles, Lip Gloss, Harmonica, BubbleZone, ntcharts). Connects to the OpenClaw Gateway via the same WebSocket protocol (v3) used by the existing webchat/CLI clients — identified as `openclaw-tui` with mode `ui`.

This is a reimplementation of the existing TypeScript/pi-tui OpenClaw TUI in Go, preserving all existing functionality while upgrading the UI with the Charm ecosystem.

---

## Existing TUI Architecture (what we're replacing)

The current TUI is built with `@mariozechner/pi-tui` (a React-like terminal UI library) and uses:
- **`GatewayClient`** class (in `method-scopes-*.js`) — WebSocket connection with device-identity auth, reconnect, challenge-response handshake
- **`GatewayChatClient`** wrapper — Higher-level RPC: `sendChat`, `abortChat`, `loadHistory`, `listSessions`, `listAgents`, `patchSession`, `resetSession`, `getStatus`, `listModels`
- **pi-tui components**: `Container`, `Box`, `Text`, `Spacer`, `Markdown`, `Editor`, `Input`, `SelectList`, `SettingsList`, `Loader`, `ProcessTerminal`, `TUI`
- **Custom components**: `ChatLog`, `CustomEditor`, `AssistantMessageComponent`, `UserMessageComponent`, `ToolExecutionComponent`, `HyperlinkMarkdown`, `FilterableSelectList`, `SearchableSelectList`, `BtwInlineMessage`

### Key Behavioral Details (from source audit)

**Client Identity:**
- Client name: `"openclaw-tui"`, mode: `"ui"` (NOT `"cli"` — the existing TUI uses UI mode)
- Capabilities: `["tool-events"]` (enables streaming tool call events)
- Protocol: v3 min/max
- Instance ID: random UUID per session

**Connection & Auth Flow:**
1. Resolve gateway URL from config (local `ws://127.0.0.1:18789` or remote `wss://`)
2. Security: rejects plain `ws://` for non-loopback hosts (requires `wss://`)
3. Auth precedence: CLI flags → env vars (`OPENCLAW_GATEWAY_TOKEN`/`OPENCLAW_GATEWAY_PASSWORD`) → config file secrets
4. Device identity: auto-loads/creates keypair, signs challenge nonce, supports device-token caching
5. Reconnect: exponential backoff starting at 1s, resets on successful connect
6. Tick watchdog: server sends periodic ticks (default 30s interval); client monitors for gaps

**Session Resolution:**
- Sessions are scoped by agent: `agent:<agentId>:<key>` format
- Special keys: `"global"`, `"unknown"` pass through
- Default session key built from `agentId` + `mainKey` config
- On agent switch, session key changes automatically

**Slash Commands (full list from source):**

| Command | Behavior |
|---|---|
| `/help` | Show slash command reference |
| `/status` | RPC `status` → formatted summary (version, channels, providers, sessions, heartbeat, models, token counts) |
| `/agent <id>` | Switch agent (changes session scope) |
| `/agents` | Open searchable agent picker overlay |
| `/session <key>` | Switch to named session |
| `/sessions` | Open filterable session picker (shows derived titles, timestamps, last message preview) |
| `/model <provider/model>` | Set model via `sessions.patch` |
| `/models` | Open searchable model picker overlay |
| `/think <level>` | Set thinking level via `sessions.patch` (levels vary by provider/model) |
| `/fast <status\|on\|off>` | Toggle fast mode |
| `/verbose <on\|off>` | Toggle verbose mode (controls tool event display: off=hidden, on=headers, full=output) |
| `/reasoning <on\|off>` | Toggle reasoning display |
| `/usage <off\|tokens\|full>` | Toggle per-response usage footer |
| `/elevated <on\|off\|ask\|full>` | Set elevated exec permissions |
| `/elev` | Alias for `/elevated` |
| `/activation <mention\|always>` | Set group activation mode |
| `/new` | Create new session with unique key (`tui-<uuid>`) |
| `/reset` | Reset current session (clears history, reloads) |
| `/abort` | Abort active chat run |
| `/settings` | Open settings overlay (tool output: collapsed/expanded, show thinking: on/off) |
| `/exit` or `/quit` | Exit TUI |
| **Gateway-registered commands** | Dynamically loaded from `listChatCommands()` — any `/command` not matched above is forwarded as a chat message |

**Special Input Prefixes:**
- `!<command>` — Local shell execution (with permission prompt on first use per session)
- `/btw:<question>` — Side-channel question (rendered in dismissable overlay, tracked separately from main chat)

**Keyboard Shortcuts:**
| Key | Action |
|---|---|
| `Enter` | Submit message |
| `Alt+Enter` | Newline in editor |
| `Ctrl+C` | Clear input → warn → exit (two-press) |
| `Ctrl+D` | Exit immediately |
| `Ctrl+L` | Open model picker |
| `Ctrl+G` | Open agent picker |
| `Ctrl+P` | Open session picker |
| `Ctrl+T` | Toggle thinking display |
| `Ctrl+O` | Toggle tool output expanded/collapsed |
| `Escape` | Dismiss BTW overlay / abort active run |
| `Shift+Tab` | (reserved, custom editor hook) |

**Chat Event Handling:**
- Events arrive as `{ event: "chat", payload: { sessionKey, runId, state, message, errorMessage } }`
- States: `delta` (streaming), `final` (done), `aborted`, `error`
- Stream assembler tracks per-run state: thinking text + content text, composed for display
- Tool events via `agent` event with `stream: "tool"`: phases `start`, `update`, `result`
- Lifecycle events via `agent` event with `stream: "lifecycle"`: phases `start`, `end`, `error`
- BTW (side results) via `chat.side_result` event
- Local run ID tracking prevents duplicate renders (server may echo back what we sent)
- Concurrent run handling: if a second run starts while first is active, history refresh is deferred

**Status Bar (footer):**
Format: `agent <id> | session <key> | <provider/model> | think <level> | fast | verbose <level> | reasoning | tokens <used>/<context> (<pct>%)`
- All flags shown only when non-default
- Dynamically updates on session info changes

**Activity Status Display:**
- Busy states: `sending`, `waiting`, `streaming`, `running` → spinner with elapsed timer
- `waiting` state has fun shimmer animation with rotating phrases ("flibbertigibbeting", "kerfuffling", "noodling", etc.)
- Idle states show `<connectionStatus> | <activityStatus>` in dim text

**History Loading:**
- On session switch: clear chat log, load history via `chat.history` (limit 200)
- Renders user messages, assistant messages (with optional thinking), tool results, command messages, system messages
- Tool results shown only when verbose mode is on

**Overlay System:**
- Single overlay at a time (managed by pi-tui's `showOverlay`/`hideOverlay`)
- Used for: agent picker, session picker, model picker, settings list, local shell permission prompt
- `SearchableSelectList`: fuzzy search with highlight, vim nav (j/k when no filter text), word-boundary scoring
- `FilterableSelectList`: simpler filter + select combo
- `SettingsList`: toggle settings with current value display

**Markdown Rendering:**
- Full markdown with syntax highlighting (via `cli-highlight` + VS Code-like theme)
- OSC 8 hyperlinks for clickable URLs in terminals that support them
- Code blocks: language detection, VS Code dark/light themes
- RTL text isolation for bidirectional content
- Binary data detection and redaction
- Long token wrapping (>32 chars) with copy-sensitivity (preserves URLs/paths)

---

## New Architecture (Bubble Tea)

```
┌─────────────────────────────────────────────────────────┐
│                    Bubble Tea Program                    │
│                                                         │
│  ┌─────────────────────────────────────────────────────┐│
│  │  Header: openclaw tui - <url> - agent - session     ││
│  ├─────────────────────────────────────────────────────┤│
│  │                                                     ││
│  │  Chat Viewport (scrollable)                         ││
│  │  ├─ system messages (dim)                           ││
│  │  ├─ user messages (bg highlight)                    ││
│  │  ├─ assistant messages (markdown rendered)           ││
│  │  └─ tool executions (collapsible boxes)             ││
│  │                                                     ││
│  ├─────────────────────────────────────────────────────┤│
│  │  Status: spinner/shimmer | elapsed | connection     ││
│  ├─────────────────────────────────────────────────────┤│
│  │  Footer: agent | session | model | flags | tokens   ││
│  ├─────────────────────────────────────────────────────┤│
│  │  Input: textarea with autocomplete + slash commands ││
│  └─────────────────────────────────────────────────────┘│
│                                                         │
│  ┌ Overlay (when active) ──────────────────────────────┐│
│  │  SearchableSelectList / SettingsList / Confirm       ││
│  └─────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────┘
         │
         │ tea.Cmd / tea.Msg
         ▼
┌─────────────────────────────────────────────────────────┐
│              Gateway WebSocket Client (Go)               │
│  Protocol v3: req/res/event frames, JSON over WS        │
│  Auth: token, password, or device-pair                   │
│  Client ID: "openclaw-tui", Mode: "ui"                  │
│  Caps: ["tool-events"]                                   │
└─────────────────────────────────────────────────────────┘
         │
         ▼
    OpenClaw Gateway (ws://127.0.0.1:18789 or remote)
```

---

## Gateway Protocol (v3) — What We Implement

### Connection Handshake
1. WebSocket connect to `ws://<host>:<port>/ws`
2. Send `ConnectParams` JSON with:
   - `minProtocol: 3, maxProtocol: 3`
   - `client: { id: "openclaw-tui", version: "0.1.0", platform: runtime.GOOS, mode: "cli" }`
   - `caps: ["tool-events"]`
   - `auth: { token: "<token>" }` (or password/bootstrapToken)
3. Receive `HelloOk` with: server info, protocol version, features (methods + events), snapshot (presence, health, session defaults, auth mode), policy (maxPayload, tick interval)

### Frame Types
| Type    | Direction       | Shape                                                      |
|---------|-----------------|------------------------------------------------------------|
| `req`   | Client → Server | `{ type: "req", id: "<uuid>", method: "<name>", params? }` |
| `res`   | Server → Client | `{ type: "res", id: "<uuid>", ok: bool, payload?, error? }`|
| `event` | Server → Client | `{ type: "event", event: "<name>", payload?, seq? }`       |

### RPC Methods Used by TUI (from source audit)

**Core (used by `GatewayChatClient` — Phase 1):**
- `connect` — Handshake with auth + device identity → `HelloOk` response
- `chat.send` — Send message, params: `{ sessionKey, message, thinking?, deliver?, timeoutMs?, idempotencyKey }`
- `chat.abort` — Abort run, params: `{ sessionKey, runId }`
- `chat.history` — Load history, params: `{ sessionKey, limit? }` → messages array + session metadata
- `sessions.list` — List sessions, params: `{ limit?, activeMinutes?, includeGlobal?, includeUnknown?, includeDerivedTitles?, includeLastMessage?, agentId? }` → sessions array + defaults
- `sessions.patch` — Update session, params: `{ key, model?, thinkingLevel?, fastMode?, verboseLevel?, reasoningLevel?, responseUsage?, elevatedLevel?, groupActivation? }` → updated entry
- `sessions.reset` — Reset session, params: `{ key, reason? }`
- `agents.list` — List agents → `{ agents, defaultId, mainKey, scope }`
- `models.list` — List models → `{ models: [{ provider, id, name }] }`
- `status` — Gateway status summary → formatted object

**Future expansion (not in existing TUI but available):**
- `sessions.create`, `sessions.delete`, `sessions.compact`, `sessions.usage`
- `config.get`, `config.patch`, `config.schema.lookup`
- `channels.status`
- `cron.list`, `cron.add`, `cron.run`
- `logs.tail`

### Events Handled by TUI (from source audit)

| Event | Handler | Purpose |
|---|---|---|
| `chat` | `handleChatEvent` | Streaming deltas, final results, aborts, errors — the core chat loop |
| `agent` | `handleAgentEvent` | Tool call events (`stream: "tool"`, phases: start/update/result) and lifecycle events (`stream: "lifecycle"`) |
| `chat.side_result` | `handleBtwEvent` | BTW side-channel results (question + answer overlay) |

The TUI also monitors connection lifecycle (onConnected, onDisconnected, onGap) but these aren't protocol events — they're WebSocket state callbacks.

---

## Component Map — Charm Libraries → UI Regions

### Bubble Tea (core framework)
- **`tea.Program`** — Root program with full-screen alt-screen, mouse support enabled
- **`tea.Model`** — Root model orchestrates: header, chat viewport, status, footer, input, overlay
- **`tea.Cmd`** — All async operations (WS send/recv, RPC calls) wrapped as commands
- **`tea.Msg`** — All WS events (`ChatEventMsg`, `AgentEventMsg`, `ConnectedMsg`, `DisconnectedMsg`) + responses (`SessionsListResult`, `ModelsListResult`) delivered as messages
- **`tea.WindowSizeMsg`** — Terminal resize triggers relayout of all components

### Bubbles (UI primitives)

| Bubbles Component     | Replaces (from pi-tui)           | Used For                                            |
|-----------------------|----------------------------------|-----------------------------------------------------|
| **`textarea`**        | `CustomEditor` + `Editor`        | Message input: multi-line, `Enter` submit, `Alt+Enter` newline, slash command autocomplete |
| **`textinput`**       | `Input` (in SelectList/Filter)   | Search/filter input in overlay pickers               |
| **`viewport`**        | `Container` (scroll in pi-tui)   | Chat log scroll area — all rendered messages         |
| **`spinner`**         | `Loader`                         | Activity spinner during busy states (sending/waiting/streaming/running) |
| **`help`**            | (none — new addition)            | Contextual keybinding hints at bottom                |
| **`key`**             | Key matching in `CustomEditor`   | Keybinding definitions for all modes                 |
| **`stopwatch`**       | Manual `formatElapsed()`         | Elapsed time display during active runs              |
| **`cursor`**          | (built into textarea)            | Blinking cursor in text inputs                       |
| **`list`**            | `SearchableSelectList`           | Overlay pickers for agents, models, sessions         |
| **`table`**           | (none — new for /status)         | Formatted status output, session list in /status     |
| **`paginator`**       | (none — new addition)            | Long history scrollback indicator                    |

### Lip Gloss (styling)

| Lip Gloss Feature     | Replaces (from pi-tui)           | Used For                                            |
|-----------------------|----------------------------------|-----------------------------------------------------|
| **`Style`**           | `chalk.hex()`, `fg()`, `bg()`    | Every text element: colors, bold, italic, dim        |
| **`NewStyle().Background()`** | `theme.userBg`, `theme.toolPendingBg` etc. | User message background, tool execution boxes |
| **`Border`**          | `Box` component                  | Tool execution boxes, overlay borders                |
| **`Color`** + `AdaptiveColor` | `isLightBackground()` detection | Dark/light theme with same palette as existing (VS Code-inspired) |
| **`JoinVertical`**    | `Container.addChild()`           | Stacking: header → chat → status → footer → input   |
| **`Place`**           | (none)                           | Centering overlay pickers                            |
| **`Width`/`Height`**  | pi-tui auto-layout               | Explicit sizing for viewport, input area             |

### Harmonica (physics-based animation)

| Harmonica Feature     | Replaces                         | Used For                                            |
|-----------------------|----------------------------------|-----------------------------------------------------|
| **`Spring`**          | `shimmerText()` manual animation | Smooth shimmer effect on waiting phrases             |
| **`Spring`**          | (new)                            | Overlay slide-in/out animation                       |
| **`Spring`**          | (new)                            | Smooth viewport scroll-to-bottom on new messages     |
| **`FPS(60)`**         | `setInterval(120ms)`             | Frame timing for waiting animation (upgrade from 120ms to 60fps) |

### BubbleZone (mouse support)

| BubbleZone Feature    | Replaces                         | Used For                                            |
|-----------------------|----------------------------------|-----------------------------------------------------|
| **`zone.Manager`**    | (none — pi-tui has no mouse)     | Root-level mouse event management                    |
| **`zone.Mark()`**     | (new)                            | Clickable tool execution headers (expand/collapse)   |
| **`zone.Mark()`**     | (new)                            | Clickable URLs in chat messages (open browser)       |
| **`zone.Mark()`**     | (new)                            | Clickable items in overlay pickers                   |
| **`zone.Mark()`**     | (new)                            | Click-to-scroll in chat viewport                     |
| **`zone.Scan()`**     | (new)                            | Root-level mouse event → zone resolution             |

### ntcharts (terminal charts)

| ntcharts Component    | Replaces                         | Used For                                            |
|-----------------------|----------------------------------|-----------------------------------------------------|
| **`Sparkline`**       | Text-only `formatTokens()`       | Inline token usage sparkline in footer bar           |
| **`BarChart`**        | (new)                            | `/status` view: session token usage comparison       |
| **`StreamlineChart`** | (new)                            | Live token consumption rate during streaming         |
| **`TimeSeriesChart`** | (new)                            | Token usage over time in enhanced `/status`          |

### Glamour (markdown rendering)

| Feature               | Replaces                         | Used For                                            |
|-----------------------|----------------------------------|-----------------------------------------------------|
| **`glamour.Render()`**| Custom `Markdown` + `HyperlinkMarkdown` | Rendering assistant message markdown content   |
| **Custom code renderer** | `cli-highlight` + `createSyntaxTheme()` | Syntax-highlighted code blocks             |
| **OSC 8 links**       | `addOsc8Hyperlinks()`            | Clickable terminal hyperlinks                       |

---

## Views & Overlays

The existing TUI has a single-view architecture (chat) with modal overlays. We preserve this model but add optional views accessible via slash commands.

### Primary: Chat View (always visible)
The main and default view. Vertical stack layout:

1. **Header line** — `openclaw tui - <gateway_url> - agent <id> (<name>) - session <key>`
2. **Chat viewport** (scrollable) — Message log containing:
   - System messages (dim text, no background)
   - User messages (markdown rendered, background-highlighted box)
   - Assistant messages (full markdown with syntax highlighting, OSC 8 links)
   - Tool executions (bordered boxes: pending=blue bg, success=green bg, error=red bg; collapsible; shows emoji+label header, args summary, output preview limited to 12 lines)
   - BTW inline messages (side-result overlay with question/answer, dismissable with Enter/Esc)
   - Command output (system-styled, from /status etc.)
3. **Status line** — Two modes:
   - **Busy**: Spinner + shimmer phrase animation + elapsed timer + connection status
   - **Idle**: `<connectionStatus> | <activityStatus>` in dim text
4. **Footer line** — `agent <id> | session <key> (<displayName>) | <provider/model> | think <level> | fast | verbose <level> | reasoning | tokens <used>/<ctx> (<pct>%)`
5. **Input area** — Multi-line textarea with slash command autocomplete

### Overlay: Searchable Picker (agents, models)
- Text input at top ("search: ")
- Fuzzy-filtered list below with highlight on matches
- Smart filtering: exact substring > word boundary > description match > fuzzy
- Vim navigation (j/k when no filter text, arrows always)
- Enter to select, Escape to cancel
- Description shown inline when width allows (≥40 chars)

### Overlay: Filterable Picker (sessions)
- Filter input at top ("Filter: ")
- Filtered list with label + description (timestamp + last message preview)
- Session search text includes: displayName, label, subject, sessionId, key, lastMessagePreview

### Overlay: Settings List
- Toggle-style settings (current value shown)
- Currently: tool output (collapsed/expanded), show thinking (on/off)
- Navigable with arrows, Enter to change, Escape to close

### Overlay: Local Shell Permission
- Yes/No selector (first `!command` per session)
- Warning about running commands on local machine

### Enhanced Views (new in Go version)

#### `/status` Enhanced View
- Everything from existing `formatStatusSummary` (version, link channel, providers, heartbeat, session store, default model, active sessions, recent sessions with token usage, queued events)
- **New**: ntcharts `BarChart` showing token usage per recent session
- **New**: ntcharts `Sparkline` in footer showing recent token consumption trend

---

## Project Structure

```
openclaw-tui/
├── main.go                     # Entry point, CLI flags (cobra), tea.Program init
├── go.mod
├── go.sum
├── PLAN.md                     # This file
│
├── internal/
│   ├── gateway/                # WebSocket client & protocol
│   │   ├── client.go           # GatewayClient: WS connection, auth, reconnect, backoff
│   │   ├── chatchlient.go      # GatewayChatClient: high-level RPC wrappers
│   │   ├── protocol.go         # Frame types (req/res/event), serialization
│   │   ├── auth.go             # Auth resolution: token/password/device-identity/env/config
│   │   ├── device.go           # Device identity: keypair gen, challenge signing, token cache
│   │   ├── events.go           # Event type definitions → tea.Msg conversions
│   │   └── connection.go       # Connection resolution: URL, TLS fingerprint, security checks
│   │
│   ├── model/                  # Bubble Tea models
│   │   ├── root.go             # Root model: layout, routing, key dispatch, overlay management
│   │   ├── chat.go             # Chat log model: message list, tool tracking, BTW
│   │   ├── input.go            # Input model: textarea + autocomplete + slash detection
│   │   ├── status.go           # Status bar: spinner/shimmer (busy) or text (idle)
│   │   ├── header.go           # Header line model
│   │   ├── footer.go           # Footer line model (flags, tokens)
│   │   ├── overlay.go          # Overlay container: manages single active overlay
│   │   ├── picker.go           # Searchable/filterable picker overlay (agents, models, sessions)
│   │   ├── settings.go         # Settings list overlay
│   │   ├── confirm.go          # Yes/No confirmation overlay (local shell permission)
│   │   └── btw.go              # BTW inline message display
│   │
│   ├── stream/                 # Chat stream processing
│   │   └── assembler.go        # TuiStreamAssembler: per-run thinking+content accumulation
│   │
│   ├── commands/               # Slash command handling
│   │   ├── registry.go         # Command registry, aliases (elev→elevated), autocomplete
│   │   ├── handlers.go         # Command dispatch: /status, /model, /think, etc.
│   │   └── session.go          # Session actions: setSession, loadHistory, refreshSessionInfo
│   │
│   ├── theme/                  # Lip Gloss theme system
│   │   ├── theme.go            # Color palette (dark+light), all style constructors
│   │   ├── detect.go           # Light/dark detection: OPENCLAW_THEME env, COLORFGBG, xterm colors
│   │   └── syntax.go           # Code syntax highlighting theme (VS Code dark/light)
│   │
│   ├── render/                 # Content rendering
│   │   ├── markdown.go         # Glamour markdown rendering with custom theme
│   │   ├── code.go             # Syntax-highlighted code blocks (chroma)
│   │   ├── hyperlinks.go       # OSC 8 hyperlinks: URL extraction, cross-line handling
│   │   ├── tool.go             # Tool execution box rendering
│   │   ├── message.go          # Message type rendering (user/assistant/system/command)
│   │   └── sanitize.go         # Text sanitization: RTL, binary, long tokens, control chars
│   │
│   ├── animation/              # Harmonica-powered animations
│   │   ├── shimmer.go          # Waiting phrase shimmer effect
│   │   └── spring.go           # Shared spring configs for overlays
│   │
│   ├── zone/                   # BubbleZone wrappers
│   │   └── manager.go          # Zone IDs for clickable regions
│   │
│   ├── charts/                 # ntcharts wrappers
│   │   ├── sparkline.go        # Token usage sparkline for footer
│   │   ├── barchart.go         # Session token comparison for /status
│   │   └── streamline.go       # Live token rate during streaming
│   │
│   ├── keymap/                 # Keybinding definitions
│   │   └── keys.go             # All keybindings, contextual help text
│   │
│   ├── shell/                  # Local shell execution
│   │   └── runner.go           # !command handling with permission gating
│   │
│   └── config/                 # TUI-local config
│       └── config.go           # ~/.config/openclaw-tui/config.yaml + env resolution
│
└── assets/                     # Embedded assets (go:embed)
    └── logo.txt                # ASCII art logo for startup
```

---

## Key Dependencies

```go
require (
    // Core TUI
    charm.land/bubbletea/v2                  // Elm Architecture TUI framework
    github.com/charmbracelet/bubbles/v2      // textarea, viewport, spinner, list, help, key, table, stopwatch
    github.com/charmbracelet/lipgloss/v2     // Styling, layout, borders, colors, adaptive profiles

    // Extensions
    github.com/charmbracelet/harmonica       // Spring physics for shimmer + overlay animations
    github.com/lrstanley/bubblezone/v2       // Mouse click zones for tool headers, URLs, picker items
    github.com/NimbleMarkets/ntcharts        // Sparkline, bar chart, streamline chart

    // Rendering
    github.com/charmbracelet/glamour/v2      // Markdown → ANSI rendering
    github.com/alecthomas/chroma/v2          // Syntax highlighting for code blocks

    // WebSocket
    github.com/gorilla/websocket             // WS client (matches gorilla used widely in Go ecosystem)

    // Crypto (device identity)
    golang.org/x/crypto                      // Ed25519 key generation + signing

    // Utility
    github.com/google/uuid                   // Request IDs, instance IDs, run IDs
    github.com/spf13/cobra                   // CLI flags: --url, --token, --password, --session, --message
    github.com/spf13/viper                   // Config file reading (~/.config/openclaw-tui/config.yaml)
)
```

---

## Implementation Phases

### Phase 1: Foundation — Connect & Chat (MVP)
Goal: Feature parity with existing TUI for basic chat.

- [ ] **Gateway WS client** — Connect, challenge-response handshake, auth (token/password), auto-reconnect with backoff
- [ ] **Protocol frames** — `req`/`res`/`event` JSON frames, request ID tracking, pending request map with timeouts
- [ ] **`GatewayChatClient` equivalent** — `sendChat`, `abortChat`, `loadHistory`, `listSessions`, `listAgents`, `patchSession`, `resetSession`, `getStatus`, `listModels`
- [ ] **Root Bubble Tea model** — Full-screen, vertical layout: header → viewport → status → footer → input
- [ ] **Chat viewport** — Scrollable message log with `viewport` bubble
- [ ] **Message rendering** — User messages (bg), assistant messages (plain text first), system messages (dim)
- [ ] **Input area** — `textarea` bubble with Enter=submit, Alt+Enter=newline
- [ ] **Stream assembler** — Per-run thinking + content text accumulation, delta/final/aborted/error states
- [ ] **Event handling** — `chat`, `agent` (lifecycle), `chat.side_result` events → `tea.Msg`
- [ ] **Header** — `openclaw tui - <url> - agent <id> - session <key>`
- [ ] **Footer** — agent | session | model | flags | token count
- [ ] **Status line** — Connection status, activity status
- [ ] **Spinner** — `spinner` bubble for busy states
- [ ] **Lip Gloss theme** — Dark/light detection, full color palette matching existing VS Code-inspired theme
- [ ] **Basic commands** — `/help`, `/new`, `/reset`, `/abort`, `/exit`, `/quit`
- [ ] **Ctrl+C handling** — Clear input → warn → exit (two-press)
- [ ] **Ctrl+D** — Exit immediately

### Phase 2: Commands & Overlays
Goal: All slash commands working, overlay pickers functional.

- [ ] **Slash command autocomplete** — Tab-completion in textarea for all `/commands`
- [ ] **Searchable select list** — Overlay component: fuzzy search, word-boundary scoring, vim nav, highlight matches
- [ ] **Agent picker** — `/agent <id>` direct + `/agents` overlay (Ctrl+G)
- [ ] **Session picker** — `/session <key>` direct + `/sessions` overlay (Ctrl+P), with derived titles + timestamps + last message preview
- [ ] **Model picker** — `/model <p/m>` direct + `/models` overlay (Ctrl+L)
- [ ] **Session patch commands** — `/think`, `/fast`, `/verbose`, `/reasoning`, `/usage`, `/elevated`, `/activation`
- [ ] **Settings overlay** — `/settings` with tool output + show thinking toggles
- [ ] **`/status` command** — Formatted gateway status summary
- [ ] **Gateway-registered commands** — Dynamic command loading, unknown `/commands` forwarded as chat messages
- [ ] **Local shell** — `!command` prefix with permission prompt overlay
- [ ] **BTW inline messages** — `/btw:` prefix, side-result rendering, Enter/Esc dismiss

### Phase 3: Rich Rendering
Goal: Full markdown, tool calls, syntax highlighting — visual parity with existing TUI.

- [ ] **Glamour markdown** — Headings, lists, bold/italic, links, blockquotes, horizontal rules
- [ ] **Syntax highlighting** — Code blocks with language detection, chroma with VS Code theme
- [ ] **OSC 8 hyperlinks** — Clickable URLs in supporting terminals, handles word-wrap splits
- [ ] **Tool execution components** — Bordered boxes with emoji+label header, args summary, output preview (12-line cap), expand/collapse (Ctrl+O)
- [ ] **Text sanitization** — RTL isolation, binary detection/redaction, long token wrapping, control char stripping, ANSI stripping
- [ ] **History rendering** — Full replay of tool results, command messages, thinking blocks
- [ ] **Elapsed timer** — Stopwatch for active runs
- [ ] **Waiting shimmer** — Harmonica spring-driven shimmer text animation with rotating phrases

### Phase 4: Mouse, Charts & Polish
Goal: Beyond parity — leverage Charm ecosystem advantages.

- [ ] **BubbleZone mouse support** — Click tool headers to toggle, click URLs to open, click overlay items
- [ ] **ntcharts sparkline** — Token usage trend in footer
- [ ] **ntcharts bar chart** — Session token comparison in `/status`
- [ ] **ntcharts streamline** — Live token rate during streaming
- [ ] **Harmonica animations** — Smooth overlay transitions, scroll momentum
- [ ] **Help bar** — Contextual keybinding hints (toggleable)
- [ ] **Viewport scrollback** — Page up/down, mouse wheel, scroll-to-bottom on new message
- [ ] **Config file** — `~/.config/openclaw-tui/config.yaml` for gateway URL, auth, theme preferences
- [ ] **Device identity** — Ed25519 keypair generation, challenge signing, device-token caching (match existing behavior)
- [ ] **TLS fingerprint verification** — For `wss://` connections with pinned certs
- [ ] **Burst coalescing** — Paste detection for Windows/Git Bash (multi-line submit coalescing)
- [ ] **Backspace deduplication** — Terminal-specific double-backspace filtering

---

## Design Decisions

1. **Why Go + Bubble Tea instead of keeping TypeScript + pi-tui?**
   - Single binary distribution (no Node.js runtime needed) — `go install` or download binary
   - Sub-100ms startup vs ~800ms for the TypeScript TUI
   - Native terminal handling — Bubble Tea's cell-based renderer is faster than pi-tui
   - Elm Architecture is a natural fit for the event-driven WS protocol
   - Mouse support (BubbleZone) — pi-tui has no mouse support at all
   - Charts (ntcharts) — token visualization not possible in current TUI

2. **Layout: Single-view + overlays (matching existing) vs multi-view**
   - The existing TUI uses a single chat view with modal overlays — this works well
   - We keep this model: overlays for pickers/settings, chat is always the primary view
   - Enhanced views (like rich `/status`) render inline in the chat viewport, not separate screens
   - This avoids the complexity of view routing while keeping the UX familiar

3. **Auth flow (matching existing exactly):**
   - Same resolution order: CLI flags → env vars (`OPENCLAW_GATEWAY_TOKEN`/`OPENCLAW_GATEWAY_PASSWORD`) → config file → device identity
   - Same security: reject plain `ws://` for non-loopback hosts
   - Same device identity: Ed25519 keypair in state dir, challenge-response signing
   - Same device token caching: stored after successful handshake, cleared on mismatch
   - Config: `~/.config/openclaw-tui/config.yaml` (or reads from OpenClaw config)

4. **Reconnection (matching existing):**
   - Exponential backoff starting at 1s, reset on successful connect
   - Auth failure detection: pause reconnect for certain error codes (token missing, pairing required, etc.)
   - Device token retry: one retry with stored device token on mismatch
   - Tick watchdog: detect server absence via tick interval
   - Visual indicator in status bar: "gateway disconnected: <reason>"

5. **Markdown rendering:**
   - Glamour for structural markdown (headings, lists, bold/italic, links, blockquotes)
   - Chroma for syntax highlighting (matching VS Code dark/light theme from existing)
   - OSC 8 hyperlinks for clickable URLs (same algorithm as existing, handles word-wrap splits)
   - Same text sanitization pipeline: RTL isolation, binary redaction, long token wrapping, control char stripping

6. **Stream assembly (matching existing):**
   - Per-run state machine tracking thinking text + content text
   - Boundary drop detection: when streaming includes tool calls, text blocks may be dropped at final — preserve streamed text
   - Local run ID tracking: prevent duplicate renders from server echo
   - Concurrent run handling: defer history refresh when multiple runs active

7. **What's new (beyond parity):**
   - Mouse support everywhere (click tool headers, URLs, picker items, scroll)
   - Token sparkline in footer (ntcharts)
   - Token bar charts in `/status` output
   - Live streaming rate chart
   - Harmonica-driven shimmer animation (smoother than existing 120ms setInterval)
   - Help bar with contextual keybinding hints
   - Potential for future: sidebar session list, split panes (Bubble Tea supports this naturally)
