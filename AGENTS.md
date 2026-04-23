# Repository Guidelines

## Project Structure & Module Organization
- `main.go` is the application entry point. It wires environment configuration, Telegram service setup, and HTTP routing.
- `pkg/` contains core modules:
  - `pkg/calculator/` for pace/time calculation logic.
  - `pkg/http/rest/` for HTTP handlers and webhook endpoints.
  - `pkg/telegram/` for Telegram bot integration.
- `cmd/wasm/main.go` builds the WebAssembly bundle used by the frontend.
- `assets/` stores static web assets (`index.html`, `json.wasm`, `wasm_exec.js`).
- Keep new domain logic under `pkg/<feature>/` and keep `main.go` focused on wiring.

## Build, Test, and Development Commands
- `go test ./ ./pkg/...` — run unit tests for non-WASM packages.
- `go build ./ ./pkg/...` — verify backend packages compile.
- `./build.sh` — copy `wasm_exec.js` and compile `cmd/wasm` into `assets/json.wasm`.
- `PORT=8080 TELEGRAM_TOKEN=... HOST=... DEBUG=true go run .` — run locally with explicit env vars.

## Coding Style & Naming Conventions
- Use standard Go formatting (`gofmt`) and imports (`goimports` if available).
- Follow idiomatic Go naming: exported identifiers in `CamelCase`, internal helpers in `camelCase`.
- Keep packages small and cohesive; prefer constructor-style initialization (for example `NewService()`).
- Use tabs/formatting produced by `gofmt`; do not hand-align spacing.

## Testing Guidelines
- Use Go’s `testing` package with table-driven tests (see `pkg/calculator/timing_test.go`).
- Test files must end with `_test.go`; test functions should be named `Test<Behavior>`.
- Add tests for every calculation or parsing branch added.
- Run tests and WASM build before opening a pull request.

## Commit & Pull Request Guidelines
- Existing history uses short, imperative commit subjects (for example: `add telegram webhook api`).
- Prefer one logical change per commit; keep subject lines concise and action-oriented.
- PRs should include:
  - purpose and scope,
  - related issue/ticket (if any),
  - test evidence (`go test ./...`, manual webhook/WASM checks),
  - screenshots only when UI changes in `assets/` are involved.

## Security & Configuration Tips
- Required runtime env vars: `PORT`, `TELEGRAM_TOKEN`, `HOST`; optional: `DEBUG`.
- Never commit real secrets or tokens. Use local shell exports or deployment secret management.

## Agent Workflow Notes
- When planning work, create or update `PLAN.md` with a TODO checklist (`[ ]` items).
- As soon as a task is completed, mark it done in `PLAN.md` (`[x]`) in the same work session.
- Add short dated status entries in `PLAN.md` when meaningful milestones are finished.
- Do not leave completed work unmarked; keep plan state synchronized with actual implementation progress.
