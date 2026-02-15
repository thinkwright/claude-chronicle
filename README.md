# Claude Chronicle (clog)

[![CI](https://github.com/thinkwright/claude-chronicle/actions/workflows/ci.yaml/badge.svg)](https://github.com/thinkwright/claude-chronicle/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/thinkwright/claude-chronicle)](https://github.com/thinkwright/claude-chronicle/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/thinkwright/claude-chronicle.svg)](https://pkg.go.dev/github.com/thinkwright/claude-chronicle)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Claude Chronicle** (clog) is a terminal dashboard for navigating, searching, and monitoring [Claude Code](https://docs.anthropic.com/en/docs/claude-code) activity on your machine.

clog reads the JSONL files Claude Code writes to `~/.claude/projects/` and presents them in a multi-pane TUI: projects, sessions, watchlist, and a full conversation log with live tailing. Memory and hooks introspection are a hotkey away.

## Install

### Go install

```bash
go install github.com/thinkwright/claude-chronicle/cmd/clog@latest
```

### Homebrew

```bash
brew install thinkwright/tap/clog
```

### Curl

```bash
curl -sSL https://thinkwright.ai/clog/install | sh
```

### Build from source

```bash
git clone https://github.com/thinkwright/claude-chronicle.git
cd claude-chronicle
make build
./clog
```

## Requirements

- Claude Code installed (clog reads from `~/.claude/projects/`)
- Go 1.25+ (only for `go install` or building from source)

## Usage

```bash
clog              # launch the dashboard
clog --reindex    # rebuild the search index from scratch
clog --version    # print version
```

## Keybindings

### Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle panes |
| `↑` / `k` | Move up / scroll up |
| `↓` / `j` | Move down / scroll down |
| `PgUp` / `PgDn` | Page up / down in detail pane |
| `g` | Jump to bottom of conversation |
| `G` | Jump to top of conversation |
| `q` | Quit (shows confirmation) |
| `Ctrl+C` | Force quit |

### Search

| Key | Action |
|-----|--------|
| `/` | Open search (project scope) |
| `Tab` | Cycle scope: project → global → local |
| `Enter` | Navigate to selected result |
| `n` / `N` | Next / previous match |
| `Esc` | Close search; press again to clear highlights |

### Filters & Watchlist

| Key | Action |
|-----|--------|
| `f` | Open filter editor |
| `F` | Clear all filters |
| `w` | Toggle watchlist pane |
| `a` / `W` | Add new watchlist pattern |

### Views

| Key | Action |
|-----|--------|
| `M` | Open Memory viewer |
| `H` | Open Hooks viewer |
| `?` | Open Settings panel |

## Features

- **Multi-pane dashboard** — projects, sessions, watchlist, and conversation detail in a split layout
- **Full-text search** — SQLite FTS5-powered search with project, global, and local scopes
- **Watchlist** — regex patterns that monitor conversations in real time with unseen match counts
- **Live tailing** — auto-scrolls as Claude Code writes; filter and traverse the conversation log
- **Memory viewer** — inspect project memory files with tab switching and markdown rendering
- **Hooks viewer** — browse Claude Code hooks configuration across global, project, and local scopes
- **Settings** — database statistics, incremental and full reindex controls
- **Structured filters** — filter by message type, model, tool, or token count
- **Zero config** — auto-discovers Claude Code projects, no setup required
- **Single binary** — pure Go, no CGO, no external dependencies

## Documentation

Full documentation available at [thinkwright.ai/clog/docs](https://thinkwright.ai/clog/docs).

## License

[MIT](LICENSE)
