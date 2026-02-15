# Releasing

## Prerequisites

- GitHub Actions secrets configured:
  - `GITHUB_TOKEN` — automatic in GitHub Actions
  - `HOMEBREW_TAP_GITHUB_TOKEN` — personal access token with write access to `thinkwright/homebrew-tap`

## Version Strategy

We follow [Semantic Versioning](https://semver.org/):

- **Patch** (0.0.x): Bug fixes, UI polish, documentation
- **Minor** (0.x.0): New features, new views, new keybindings
- **Major** (x.0.0): Breaking changes to CLI flags, config format, or data schema

## Release Process

1. **Verify main is clean**

   ```bash
   git checkout main
   git pull
   make test
   make lint
   ```

2. **Update CHANGELOG.md**

   Move items from `[Unreleased]` to a new versioned heading:

   ```markdown
   ## [0.1.0] - 2026-02-15
   ```

3. **Commit the changelog**

   ```bash
   git add CHANGELOG.md
   git commit -m "Release v0.1.0"
   ```

4. **Tag the release**

   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin main
   git push origin v0.1.0
   ```

5. **GitHub Actions takes over**

   The `release.yaml` workflow triggers on the tag push:
   - Runs tests
   - Executes GoReleaser for cross-platform builds
   - Creates a GitHub release with binaries and checksums
   - Updates the Homebrew tap formula

6. **Verify the release**

   - Check [GitHub Releases](https://github.com/thinkwright/claude-chronicle/releases)
   - Verify Homebrew: `brew install thinkwright/tap/clog`
   - Verify go install: `go install github.com/thinkwright/claude-chronicle/cmd/clog@latest`
   - Verify curl installer: `curl -sSL https://thinkwright.ai/clog/install | sh`

## Build Targets

| Archive | OS | Arch |
|---------|----|------|
| `claude-chronicle_linux_amd64.tar.gz` | Linux | x86_64 |
| `claude-chronicle_linux_arm64.tar.gz` | Linux | ARM64 |
| `claude-chronicle_darwin_amd64.tar.gz` | macOS | Intel |
| `claude-chronicle_darwin_arm64.tar.gz` | macOS | Apple Silicon |
| `claude-chronicle_windows_amd64.zip` | Windows | x86_64 |
| `claude-chronicle_windows_arm64.zip` | Windows | ARM64 |

## Rollback

If a release has issues:

1. Delete the git tag: `git tag -d v0.1.0 && git push origin :v0.1.0`
2. Delete the GitHub release from the web UI
3. Fix the issue, commit, and re-release with the same or bumped version
