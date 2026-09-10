# Implementation Plan

## Summary
- Priority: stabilize and upgrade Go/WASM/build first, then refresh UI without product rewrite.
- UI approach: keep `Vanilla JS + modern CSS` (no SPA framework in this cycle).
- Deployment target: local + Docker compatibility.
- Quality gates: `go test ./...`, `go vet ./...`, WASM build, manual UI smoke checks.

## Work Items

### 1) Runtime and Build Upgrade
- [x] Upgrade Go version in `go.mod` and Docker base image.
- [x] Make WASM build reproducible (`wasm_exec.js` + `json.wasm` generation via `build.sh`).
- [x] Keep startup/config behavior explicit and documented.
- [x] Ensure README contains canonical `build`, `test`, and `run` commands.

### 2) UI Refresh (No Breaking Integration)
- [x] Improve visual hierarchy, spacing, contrast, and interactive states.
- [x] Make calculator layout fully responsive.
- [x] Add user-facing validation/error feedback for invalid or empty input.
- [x] Keep existing WASM integration contract (`calcPace`, `calcTime`) unchanged.

### 3) Reliability and Operations Additions
- [x] Add CI checks for test/vet/build/wasm/docker.
- [x] Add health endpoint (`GET /healthz`) for liveness checks.
- [x] Standardize API error responses in debug endpoints.
- [x] Reduce fragile error handling in HTTP + JS↔WASM boundary.

### 4) IDE and Build Target Compatibility
- [x] Add explicit build constraints for `cmd/wasm` (`js/wasm`) to avoid host-OS IDE false errors on `syscall/js`.
- [x] Add non-WASM fallback entrypoint in `cmd/wasm` to avoid IDE/run-config failure: `build constraints exclude all Go files`.

### 5) Local Development Environment Loading
- [x] Load runtime config from environment variables, with optional `.env` fallback when file exists.
- [x] Keep environment variables priority over `.env` values for safe overrides in CI/prod.
- [x] Add unit tests for `.env` loader behavior (missing file, value load, env override precedence).

### 6) Documentation Clarity for Run Modes
- [x] Rewrite `README.md` in Russian with explicit local vs production flow.
- [x] Document separate backend startup and WASM build pipeline.
- [x] Add practical IDE-first startup guide using `.env`.

### 7) Production Container Hardening
- [x] Rewrite Dockerfile to multi-stage build with WASM compilation inside image build.
- [x] Remove misleading runtime config via Docker build args and rely on runtime env only.
- [x] Add `.dockerignore` to prevent secrets/dev artifacts from entering image context.

### 8) UI Layout and Input Ergonomics
- [x] Center app card on desktop viewport and keep mobile-friendly layout behavior.
- [x] Size numeric inputs according to realistic max digit lengths (hours/pace up to 3 digits).
- [x] Add bright-blue active state for fixed distance preset buttons, synchronized with slider/manual distance input.

### 9) Logging Modernization
- [x] Replace ad-hoc `log` usage with structured `log/slog` logging and leveled output.
- [x] Remove `panic`/`Fatal` from library packages (`pkg/...`) and return errors to `main`.
- [x] Add optional `LOG_LEVEL` env override with sane fallback to `DEBUG` behavior.

### 10) Graceful Shutdown
- [x] Add OS signal handling (`SIGINT`/`SIGTERM`) in `main` and stop server via `http.Server.Shutdown`.
- [x] Add managed Telegram service lifecycle with `Close(ctx)` to stop polling and worker goroutines.
- [x] Ensure coordinated shutdown sequence logs completion and error states.

### 11) GitHub Workflows Refresh
- [x] Consolidate CI and Docker workflows into one maintained workflow file.
- [x] Update action versions to current majors and remove deprecated workflow patterns.
- [x] Align triggers with active branches (`master` + `main`) and tagged release publishing (`v*`).

## Roadmap 2026-09 (по итогам ревизии репозитория)

Источник: аудит репозитория, разбор конкурентов и анализ WASM-стека.
Монетизация из плана исключена по решению владельца проекта.

### 12) Этап 1. Гигиена
- [x] Разделить сломанный `<meta charset ... name="viewport">` в `assets/index.html` на два тега.
- [x] Свести интерфейс к одному языку и выставить корректный `<html lang>`.
- [x] Убрать Bootstrap 4, jQuery и ion-rangeslider; перейти на нативный `input[type=range]` и CSS Grid.
- [x] Починить сообщение об ошибке в `handleTimeCmd` (печатает `arguments[1]` вместо `arguments[0]`).
- [x] Починить недостижимую ветку `"field is required"` в `pkg/http/rest/handler.go`.
- [x] Убрать бессмысленный `% 60` для часов в `cmd/wasm/main.go`.
- [x] Отвечать на неизвестные команды подсказкой вместо молчаливого приветствия.
- [x] Добавить `go test -race` и golangci-lint в CI.
- [x] Добавить файл лицензии.
- [x] Добавить Dependabot для Go-модулей и GitHub Actions.

### 13) Этап 2. Фундамент
- [ ] Перевести сборку WASM на TinyGo (`go:wasmexport`, reactor-режим).
- [x] Исполнять тот же артефакт на сервере через wazero.
- [x] Мигрировать с `go-telegram-bot-api v4` на `go-telegram/bot` (Bot API 10.3).
- [x] Заменить `NYTimes/gziphandler` на `klauspost/compress/gzhttp`.
- [x] Вынести `/api/v1` из-под флага `DEBUG`.
- [x] Добавить состояние расчёта в URL (shareable-ссылка).
- [x] Покрыть тестами `pkg/telegram` (сейчас 0%).

### 14) Этап 3. Продукт
- [ ] Telegram Mini App с авторизацией через `initData`.
- [ ] Таблица сплитов.
- [ ] Предсказание времени по формулам Ригеля и Кэмерона.
- [ ] VDOT и тренировочные зоны по Дэниелсу.
- [ ] Карточка результата картинкой в чат.
- [ ] SQLite и история расчётов.
- [ ] Тёмная тема и поддержка `themeParams` Telegram.

### 15) Этап 4. Фронтир
- [ ] Описать контракт JS<->WASM в `.wit`.
- [ ] Собрать компонент `wasip2` и сгенерировать браузерную сторону через `jco transpile`.
- [ ] Разбор GPX/FIT в браузере с поправкой на рельеф.

## Validation
- [x] Run: `go test ./ ./pkg/...`
- [x] Run: `go vet ./ ./pkg/...`
- [x] Run: `./build.sh`
- [x] Run: `go build ./ ./pkg/...`
- [ ] Manually verify distance presets, slider sync, pace/time recalculation, and invalid input handling.

## Status Log
- 2026-04-23: Added plan file and started implementation.
- 2026-04-23: Upgraded Go baseline to `1.26` and refreshed Docker/build scripts.
- 2026-04-23: Implemented UI refresh and client-side validation/error states.
- 2026-04-23: Added `/healthz`, standardized debug endpoint errors, and added HTTP handler tests.
- 2026-04-23: Added CI workflow and completed local automated checks.
- 2026-04-23: Added `js/wasm` build tags in `cmd/wasm/main.go` to fix IDE `syscall/js` highlighting on non-WASM targets.
- 2026-04-23: Added `cmd/wasm/main_nowasm.go` fallback for non-WASM runs to prevent `build constraints exclude all Go files`.
- 2026-04-23: Added optional `.env` loading in `main.go` with env-first precedence and tests in `main_test.go`.
- 2026-04-23: Rewrote `README.md` in Russian with clear local/production run and WASM build instructions.
- 2026-04-23: Hardened container build: multi-stage Dockerfile with in-build WASM generation and added `.dockerignore`.
- 2026-04-23: Validated Docker build end-to-end and fixed cross-arch backend build via `TARGETARCH/TARGETOS`.
- 2026-04-23: Updated UI layout centering, numeric input widths, and preset button active highlighting in `assets/index.html`.
- 2026-04-23: Fixed preset highlight stability for formatted slider values and made time/pace inputs compact-width on desktop.
- 2026-04-23: Increased slider max distance from `160934` to `200000` (200 km).
- 2026-04-23: Replaced time/pace numeric fields with spin controls (`+/-`, wheel, pointer drag for mouse/touch) while preserving existing input IDs and WASM contract.
- 2026-04-23: Modernized logging with `slog`, added log levels, and removed panic/fatal from internal packages in favor of error propagation.
- 2026-04-23: Implemented graceful shutdown for HTTP server and Telegram service with signal handling and bounded shutdown timeout.
- 2026-04-23: Fixed webhook URL building for hosts with/without scheme and restored `DEBUG` in `.env.example` for local polling startup.
- 2026-04-23: Refreshed GitHub workflows: merged into single `ci.yml`, updated action versions, and aligned branch/tag triggers.
- 2026-04-23: Boosted distance preset visual feedback by removing Bootstrap `disabled` dimming and strengthening active/pressed highlight styles.
- 2026-09-10: Ревизия репозитория: аудит, разбор конкурентов, план на 4 этапа (монетизация исключена).
- 2026-09-10: Этап 1 — разделён сломанный тег `meta` (charset и viewport были в одном элементе), интерфейс переведён полностью на русский, `lang="ru"`.
- 2026-09-10: Этап 1 — убраны Bootstrap 4, jQuery и ion-rangeslider; слайдер на нативном `input[type=range]`, вёрстка на flex/CSS. Страница больше не обращается к внешним CDN.
- 2026-09-10: Этап 1 — исправлены 4 дефекта: аргумент в ошибке `handleTimeCmd`, недостижимая ветка `field is required`, `%60` на часах в WASM (вынесено в `calculator.Split`), ответ на неизвестные команды.
- 2026-09-10: Этап 1 — добавлены тесты `pkg/telegram` (было 0%), тесты `Split` и HTTP-валидации; в CI добавлены `go test -race` и golangci-lint; добавлены LICENSE (MIT) и Dependabot.
- 2026-09-10: Этап 2 — `NYTimes/gziphandler` заменён на `klauspost/compress/gzhttp` под характеризующими тестами.
- 2026-09-10: Этап 2 — `/time` и `/pace` стали `/api/v1/time` и `/api/v1/pace`, работают в обоих режимах и отвечают JSON.
- 2026-09-10: Этап 2 — `cmd/calcwasm` + `pkg/wasmcalc`: тот же `pkg/calculator` исполняется на сервере через wazero (`CALC_ENGINE=wasm`), эквивалентность нативному движку покрыта тестом.
- 2026-09-10: Этап 2 — миграция с `go-telegram-bot-api v4` на `go-telegram/bot` v1.25 (Bot API 10.3); покрытие `pkg/telegram` 0% -> 65.9%, появился сквозной тест отправки через httptest.
- 2026-09-10: Этап 2 — состояние расчёта попадает в URL (`?d=&t=`) через `replaceState`, ссылка восстанавливает расчёт при открытии; добавлена кнопка копирования.
