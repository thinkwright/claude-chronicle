# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.2] - 2026-02-16

### Fixed
- System messages now render in conversation log (parseSystemMessage was not unmarshalling content)
- Hyphenated project names display correctly (decodePath validates against filesystem with cwd fallback)
- Watchlist backfill no longer races with concurrent indexing (runs synchronously with proper error handling)
- Word-wrap no longer panics on multi-byte UTF-8 content (emoji, CJK, accented characters)
- Text truncation uses rune-safe slicing instead of byte slicing

### Improved
- Project discovery skips JSON parsing after metadata found (~99% less work on large sessions)
- Animation tick rate reduced from 50ms to 500ms (~90% less idle CPU)
- Database reset uses DROP TABLE instead of per-row DELETE (faster --reindex)
- Deduplicated word-wrap logic between detail and memory views

## [0.1.1] - 2026-02-15

### Added
- Watchlist enter key filters sessions pane to matching sessions
- Live watchlist matching on incremental and full index
- Delete watch confirmation prompt (y/n)
- Case-insensitive regex hint in watch editor

### Fixed
- Auto-resize terminal to minimum 120×40 on launch
- Homebrew formula renamed to `clog` to match binary

## [0.1.0] - 2026-02-15

### Added
- Multi-pane terminal dashboard (projects, sessions, watchlist, conversation log)
- Full-text search powered by SQLite FTS5 with three scopes (project, global, local)
- Regex watchlist with real-time match counting and unseen indicators
- Live tailing of active Claude Code sessions
- Structured filters (type, model, tool, token count)
- Memory viewer modal for project memory files
- Hooks viewer modal for Claude Code hooks configuration
- Settings panel with database statistics and reindex controls
- Stale index warning with refresh hint
- Quit confirmation dialog
- Incremental and full reindex support
- Zero-config auto-discovery of Claude Code projects
- Cross-platform support (macOS, Linux, Windows)
