# AGENT STATE

## 1. Current Goal
Maintain, refactor, and scale Scribo (Go Edition) as a high-performance, ultra-lightweight, 100% JSON-driven Telegram Voice Analysis Bot with automated CI/CD releases and zero-code distribution.

## 2. Completed Steps
- 2026-07-21 18:17 - Fully rewritten Scribo in Go (Golang) with zero external dependencies, 8.9 MB static binary, and ~8-12 MB RAM footprint. Passed unit test parity.
- 2026-07-21 18:38 - Added `DEFAULT_PROVIDER` environment variable support (`google` vs `openrouter`) in `config/config.go` with 100% explicit model mapping.
- 2026-07-21 18:50 - Created executable `setup_service.sh` for 1-command 7/24 Systemd service setup on Linux / Oracle Cloud VPS.
- 2026-07-21 19:29 - Performed comprehensive multi-perspective architectural overhaul based on subagent reviews:
  - **Architecture:** Introduced `AIProvider` interface (`Generate(ctx, prompt, audio)`), decoupled `GoogleProvider` and `OpenRouterProvider`, added fail-fast `cfg.Validate()`.
  - **DevOps:** Implemented Graceful Shutdown (`signal.NotifyContext`), binary size optimization via `-ldflags "-s -w"` (reduced to ~6.4 MB), structured logging via `log/slog`, and Systemd security hardening (`ProtectSystem=full`, `PrivateTmp=yes`, `NoNewPrivileges=yes`).
  - **Telegram UX:** Added support for Voice Notes (`.ogg`), Audio Files (`.mp3`, `.m4a`), and Document audio up to 20 MB; added real-time Telegram `typing` status ticker; added Rich Text / HTML message formatting.
- 2026-07-21 19:35 - Transformed mode management to be 100% JSON-driven. Embedded core 3 default modes (`tldr`, `trans`, `fix`) in `mode/default_modes.json` via `//go:embed`, with `modes.json` on disk serving as single source of truth when present.
- 2026-07-21 19:46 - Added `make release` command to `Makefile` for zero-code distribution packaging (`scribo-linux-amd64.tar.gz` and `scribo-linux-arm64.tar.gz`). Added MIT `LICENSE` and `.env.example` template with auto-copy logic in `setup_service.sh`.
- 2026-07-21 20:07 - Configured GitHub Actions CI/CD workflow (`.github/workflows/release.yml`) with automated semantic versioning (`mathieudutour/github-tag-action@v6.2`) and GitHub Releases assets publishing (`softprops/action-gh-release@v2`).
- 2026-07-21 23:19 - Completed Senior-level Production Architecture Overhaul & Hermetic Unit Test Suite:
  - **Worker Pool & Concurrency Limiting:** Added `MAX_CONCURRENT_JOBS` configuration and counting semaphore (`workerSem`) to prevent RAM spikes under high traffic.
  - **Graceful Shutdown & Context Isolation:** Integrated `sync.WaitGroup` with detached worker context (`context.WithoutCancel`) so in-flight tasks finish gracefully without context cancellation crashes on SIGTERM.
  - **Exponential Backoff & Mockable DI:** Implemented exponential retries (up to 2 retries with timer cleanup & jitter) for transient HTTP 429/5xx errors. Made `BaseURL` and `http.Client` injectable for `httptest.NewServer` unit tests.
  - **Hermetic Unit Test Suite:** Created `config_test.go`, `mode_test.go`, `provider_test.go`, and `bot_test.go`. All tests pass cleanly with `go test -count=1 -race ./...`.

## 3. Key Conventions & Guidelines for Future AI Agents
- **Code Comments:** Write clean, concise English comments only where necessary in Go source code files.
- **Commit Messages & Version Bumping:**
  - Standard commit/push to `main` -> Auto-increments Patch version (`v0.0.1` ➔ `v0.0.2`).
  - Commit message containing `feat:` -> Auto-increments Minor version (`v0.1.0`).
  - Commit message containing `#major` or `BREAKING CHANGE:` -> Auto-increments Major version (`v1.0.0`).
- **Single Source of Truth:** Keep prompts and button titles in `mode/default_modes.json` and `modes.example.json`. Do NOT hardcode prompts inside Go code files.
- **Testing & Release:** Always run `make test` before pushing. Use `make release` for packaging binary archives.

## 4. Next Steps
- Continue supporting community feature requests and model updates.

## 5. Known Issues & Roadblocks
- None.
