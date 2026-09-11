# Repository Guidelines

## Project Structure & Module Organization
- `main.go` is the entry point: environment configuration, the calculation engine, the optional history database, the Telegram service, the HTTP server and graceful shutdown. Keep it focused on wiring.
- `pkg/` contains the modules:
  - `pkg/calculator/` — pure formulas with no I/O: pace and time, even splits, Riegel and Cameron predictions, Daniels VDOT and training paces.
  - `pkg/analysis/` — response shapes for splits, predictions and VDOT, shared by the HTTP API and the browser wasm so both return identical JSON.
  - `pkg/http/rest/` — HTTP API (`/api/v1/...`), health check, Telegram webhook, the static files and the page itself, whose link preview tags carry the plan from a shared link (`preview.go`).
  - `pkg/telegram/` — bot commands on `github.com/go-telegram/bot`, with bounded queues and worker goroutines.
  - `pkg/miniapp/` — validation of Telegram Mini App init data.
  - `pkg/history/` — saved runs in SQLite (`modernc.org/sqlite`, no cgo).
  - `pkg/wasmcalc/` — runs `calc.wasm` on the server through wazero when `CALC_ENGINE=wasm`.
- `cmd/wasm/` — browser bundle (`js && wasm`), built with TinyGo into `assets/json.wasm`.
- `cmd/calcwasm/` — server WASI reactor (`wasip1`, `go:wasmexport`), built into `pkg/wasmcalc/calc.wasm`.
- `assets/` — the page (`index.html`), `json.wasm` and the `wasm_exec.js` of the toolchain that built it, the web app manifest, the service worker (`sw.js`) and the icons, whose PNGs are rendered from `icon.svg`.
- Keep new domain logic under `pkg/<feature>/`.

## Build, Test, and Development Commands
- `go test -race ./ ./pkg/...` — unit tests.
- `go vet ./ ./pkg/...` — static checks.
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.0 run ./...` — lint with the CI version, built by the local toolchain.
- `go build ./...` — compile everything, including the host fallbacks of the wasm commands.
- `./build.sh` — rebuild both wasm artifacts: TinyGo for the browser, the standard toolchain for the server reactor; `WASM_COMPILER=go ./build.sh` builds the browser bundle with the standard toolchain. Run it after changing `cmd/wasm`, `cmd/calcwasm`, `pkg/calculator` or `pkg/analysis`, and commit the artifacts.
- `PORT=8080 TELEGRAM_TOKEN=... HOST=... DEBUG=true go run .` — run locally. The server contacts Telegram at startup, so it needs a real token; `DEBUG=true` deletes the bot's webhook, so use a separate test bot.
- `python3 -m http.server 8080 -d assets` — serve only the page; the calculator runs entirely in the browser.

## Coding Style & Naming Conventions
- Use standard Go formatting (`gofmt`) and imports (`goimports` if available).
- Follow idiomatic Go naming: exported identifiers in `CamelCase`, internal helpers in `camelCase`.
- Keep packages small and cohesive; prefer constructor-style initialization (for example `NewService()`).
- Library packages under `pkg/` return errors; only `main` exits. Log with `log/slog` key-value attributes.
- Use tabs/formatting produced by `gofmt`; do not hand-align spacing.
- Page text lives in the `messages` dictionary in `assets/index.html`: add every new string to both `ru` and `en` under the same key, and mark static markup with `data-i18n` (text) or `data-i18n-label` (`aria-label`). Bot replies live in `pkg/telegram/messages.go`.

## Testing Guidelines
- Use Go's `testing` package with table-driven tests (see `pkg/calculator/timing_test.go`).
- Test files end with `_test.go`; test functions are named `Test<Behavior>`.
- Write the failing test first, then the code.
- Check formulas against published reference values, not against the implementation itself: the VDOT tests use Daniels' table, the Mini App tests use Telegram's published init data example.
- Test HTTP handlers through `httptest`. The Telegram client can be tested without a token via `bot.WithSkipGetMe()` and `bot.WithServerURL()`; note that it sends `multipart/form-data`, not JSON.
- For page changes, check a browser at 390 px width, in both colour schemes and in both languages.
- `assets_test.go` serves every file `index.html` links through the real handler and checks its `Content-Type`; list a newly linked file there. A file the page needs offline also goes into `SHELL` in `assets/sw.js`.
- CI does not run on branch pushes, so build the image locally with `docker build .` before a pull request. The browser wasm exports can be called under Node through `assets/wasm_exec.js`.
- Run tests, lint and `./build.sh` before opening a pull request.

## Commit & Pull Request Guidelines
- Commit subjects are short and imperative (for example: `add telegram webhook api`); the body explains why.
- Prefer one logical change per commit.
- PRs should include:
  - purpose and scope,
  - related issue/ticket (if any),
  - test evidence (`go test -race`, lint, `./build.sh`, browser checks for page changes),
  - screenshots when the page in `assets/` changes.

## Security & Configuration Tips
- Required runtime env vars: `PORT`, `TELEGRAM_TOKEN`, `HOST`; optional: `DEBUG`, `LOG_LEVEL`, `CALC_ENGINE`, `DB_PATH`.
- Mini App requests authenticate with `Authorization: tma <initData>`; the server validates the signature with the bot token and accepts data for 24 hours.
- Never put the URL hash into a share link: inside Telegram it holds the user's signed init data.
- Build link preview tags from parsed numbers only; never copy a query value into the page markup.
- Never commit real secrets or tokens. Use local shell exports or deployment secret management.

## Agent Workflow Notes
- When planning work, create or update `PLAN.md` with a TODO checklist (`[ ]` items). The plan is kept in Russian.
- As soon as a task is completed, mark it done in `PLAN.md` (`[x]`) in the same work session.
- Add short dated status entries in `PLAN.md` when meaningful milestones are finished.
- Do not leave completed work unmarked; keep plan state synchronized with actual implementation progress.
