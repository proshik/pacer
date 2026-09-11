# Implementation Plan

## Summary
- Состояние на 2026-09-11: исходный план (апрель 2026, разделы 1–11) выполнен; этапы 1 и 2 закрыты;
  этап 3 закрыт, кроме живого запуска Mini App и карточки результата; к этапу 4 не приступали.
- Ждёт владельца проекта: настройка Mini App в BotFather и живой запуск; решение по шрифту с
  кириллицей для карточки результата; ревью и мерж ветки `stage-1-hygiene`.
- Интерфейс: Vanilla JS и CSS без фреймворков; вне Telegram страница не делает внешних запросов.
- Развёртывание: Docker-образ со стадией TinyGo для браузерного бандла, история расчётов в томе `/data`.
- Ворота качества: `go test -race`, `go vet`, golangci-lint v2.13, `./build.sh` на TinyGo, проверка
  страницы в браузере на 390 px, в обеих темах, в обоих языках и без сети; `docker build .` —
  локально, потому что CI для веток не запускается.

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
- [x] Перевести сборку WASM на TinyGo: браузерный бандл — на TinyGo (0,93 МБ вместо 4,54 МБ, ответы побайтно совпадают), серверный reactor с `go:wasmexport` остаётся на обычном Go.
- [x] Исполнять тот же артефакт на сервере через wazero.
- [x] Мигрировать с `go-telegram-bot-api v4` на `go-telegram/bot` (Bot API 10.3).
- [x] Заменить `NYTimes/gziphandler` на `klauspost/compress/gzhttp`.
- [x] Вынести `/api/v1` из-под флага `DEBUG`.
- [x] Добавить состояние расчёта в URL (shareable-ссылка).
- [x] Покрыть тестами `pkg/telegram` (сейчас 0%).

### 14) Этап 3. Продукт
- [x] Telegram Mini App: проверка `initData` на сервере (`pkg/miniapp`, `/api/v1/me`) и интеграция на странице — тема, вход, SDK только внутри Telegram.
- [ ] Настроить Mini App в BotFather и проверить живой запуск; он же окончательно подтвердит, что поле `signature` участвует в проверке `hash`.
- [x] Таблица сплитов.
- [x] Предсказание времени по формулам Ригеля и Кэмерона.
- [x] VDOT и тренировочные зоны по Дэниелсу.
- [ ] Карточка результата картинкой в чат.
- [x] SQLite и история расчётов на сервере: `pkg/history`, `/api/v1/runs` (список, сохранение, удаление), `DB_PATH`, том `/data` в образе.
- [x] Сохранённые расчёты на странице Mini App: кнопка «Сохранить», список «новые сверху» с темпом и датой, возврат расчёта в калькулятор, удаление.

### 14.1) Этап 3. UI и UX
Цель: интерфейс, который выглядит спроектированным под беговой калькулятор, а не собранным из шаблона, и на телефоне в Telegram удобнее конкурентов.

Процесс
- [x] Перед любой правкой UI загружать скилл `frontend-design`; сначала дизайн-план (палитра, типографика, сетка), потом код.
- [x] Избегать шаблонной «AI-эстетики»: градиентные hero-блоки, Inter/Space Grotesk по умолчанию, эмодзи вместо маркеров, карточки внутри карточек, одинаковые скругления и тени на всём подряд.
- [x] Перед коммитом UI — проверка в браузере: обе темы, ширина 400 px, консоль без ошибок.
- [x] Ограничение: `calcTime()` читает и пишет поля по id — при редизайне либо сохранить id, либо сначала поменять контракт JS↔WASM.

Иерархия и компоновка
- [x] Результат — главный элемент экрана: крупные цифры с `tabular-nums`, поля ввода вторичны.
- [x] Mobile-first: основной сценарий — Telegram на телефоне; тач-цели не меньше 44 px.
- [x] Показать на странице таблицу сплитов, прогноз по Ригелю и Кэмерону, VDOT и тренировочные зоны (данные из `/api/v1/splits`, `/api/v1/predict`, `/api/v1/vdot`).

Ввод и состояния
- [x] Состояние загрузки WASM (сейчас 3 МБ) вместо молча заблокированных полей.
- [ ] Оценить ввод времени одной строкой (`3:10:49`) вместо трёх спиннеров. Пока оставлены сегменты-колёсики: они дают ввод без клавиатуры на телефоне.
- [x] Понятные ошибки валидации рядом с полем, а не одной строкой внизу.

Темы, доступность, окружение
- [x] Тёмная тема по `prefers-color-scheme` в вебе.
- [x] `themeParams` Telegram: из хэша сразу, затем из SDK и по событию `themeChanged`.
- [x] Доступность: `aria-live` на результате, управление спиннерами с клавиатуры, видимый фокус.
- [x] PWA: манифест, иконки и service worker для офлайна на старте забега (сначала сеть, без сети — кэш).
- [x] Превью ссылки: сервер пишет в `og:`-теги план из самой ссылки («Марафон за 3:44:20 — это 5:19 на километр») на языке того, кто делится.
- [x] Локализация: страница и бот на русском и английском; язык — выбор вручную, затем `language_code` из Telegram, затем язык браузера.

### 15) Этап 4. Фронтир
- [ ] Описать контракт JS<->WASM в `.wit`.
- [ ] Вынести сплиты, прогноз и VDOT в контракт wasm-модуля: сейчас `/api/v1/splits`, `/api/v1/predict` и `/api/v1/vdot` считают нативно, и `CALC_ENGINE` на них не действует.
- [ ] Собрать компонент `wasip2` и сгенерировать браузерную сторону через `jco transpile`.
- [ ] Разбор GPX/FIT в браузере с поправкой на рельеф.

## Validation
- [x] Run: `go test -race ./ ./pkg/...`
- [x] Run: `go vet ./ ./pkg/...`
- [x] Run: golangci-lint v2.13 (`go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.0 run ./...`)
- [x] Run: `./build.sh` (TinyGo для браузера, обычный Go для reactor)
- [x] Run: `go build ./...`
- [x] Headless Chrome: 390 px без горизонтального скролла, обе темы, консоль чистая, Tab только по цифрам.
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
- 2026-09-10: Этап 3 — в `pkg/calculator` добавлены равномерные сплиты (`EvenSplits`), прогноз по Ригелю и Кэмерону, VDOT по Дэниелсу–Гилберту с обратным расчётом и тренировочными зонами E/M/T/I; всё сверено с опубликованной таблицей VDOT. В API и интерфейс пока не выведено.
- 2026-09-10: Этап 3 — формулы выведены в API: `/api/v1/splits` (не больше 500 отметок), `/api/v1/predict` (Ригель и Кэмерон на 6 дистанций), `/api/v1/vdot` (VDOT, зоны, эквиваленты); длительности отдаются как `{text, seconds}`. Эндпоинты считают нативно, мимо `CALC_ENGINE`. Добавлен раздел плана 14.1 «UI и UX».
- 2026-09-10: Этап 3 — общий слой `pkg/analysis`: API и браузерный WASM отдают одни и те же структуры; в браузер экспортированы `calcSplits` (принимает время забега, чтобы финиш совпадал с показанным временем), `calcPredict`, `calcVDOT`. Установлено: рост `json.wasm` с 3,17 до 4,5 МБ дал тулчейн Go 1.27.1, а не код (тот же исходник из ранней ревизии сегодня собирается в 4,5 МБ; `-s -w` экономит лишь 82 КБ).
- 2026-09-10: Этап 3 — редизайн страницы по скиллу `frontend-design`: палитра шоссейного бега (асфальт, бумага, жёлтая разметка только как метка), системный шрифт (как в Telegram, ноль байт, офлайн), время и темп — главное на экране с пометкой рассчитанного числа; на странице раскладка, прогноз и VDOT из браузерного WASM; тёмная тема, состояние загрузки, управление с клавиатуры. Проверено headless Chrome через DevTools Protocol: 390 px без горизонтального скролла, обе темы, консоль чистая, финиш раскладки совпадает со временем в обе стороны расчёта.
- 2026-09-10: Этап 3 — Mini App: свой валидатор `initData` (`pkg/miniapp`) вместо `bot.ValidateWebappRequest`, эндпоинт `/api/v1/me` с `Authorization: tma <initData>` и сроком 24 ч; страница подключает SDK только внутри Telegram, берёт тему и вход из хэша, не дожидаясь SDK; ссылка для шаринга больше не уносит подписанный `initData`. SDK в headless Chrome не загрузился — его ветку (`ready`, `expand`, `themeChanged`) подтвердит только живой запуск.
- 2026-09-10: Этап 3 — история расчётов на сервере: `pkg/history` на `modernc.org/sqlite` (без CGO, одно соединение — очередь вместо «database is locked», проверено 16 параллельными записями под `-race`), `GET/POST /api/v1/runs` и `DELETE /api/v1/runs/{id}` только с `Authorization: tma`, база по `DB_PATH` (в образе `/data/pacer.db`, том `/data`). На странице ещё не выведено.
- 2026-09-10: Этап 3 — сохранённые расчёты на странице Mini App: «Сохранить» становится главной кнопкой, ссылка — второстепенной; тап по записи возвращает расчёт в калькулятор. Вне Telegram и при 503 (нет `DB_PATH`) раздел скрыт. Проверено в headless Chrome с поддельным API через `Fetch`: тела и заголовки запросов, восстановление расчёта, удаление, 390 px без горизонтального скролла, консоль чистая.
- 2026-09-10: Этап 2 — браузерный бандл собирается TinyGo 0.42: `json.wasm` 928 273 байта вместо 4 536 887 (brotli 257 КБ вместо 917 КБ); ответы `calcPace`, `calcSplits`, `calcPredict`, `calcVDOT` и ошибок совпадают побайтно с обычной сборкой, страница проверена в headless Chrome. `WASM_COMPILER=go` — явный запасной путь; TinyGo добавлен в CI и Docker.
- 2026-09-10: Сборка wasm воспроизводима: reactor собирается с `-trimpath -buildvcs=false` — раньше Go вшивал ревизию git, и `calc.wasm` менялся с каждым коммитом без изменений кода. Две сборки подряд дают побайтно одинаковые файлы.
- 2026-09-10: Правки по отзыву: подчёркивание рассчитанного числа — одной линией; ширину разрядов задают цифры, стрелки вынесены из потока; «/км» на базовой линии; Tab ходит только по цифрам, при фокусе число выделяется; у поля метров нет браузерных стрелок; лента пресетов на телефоне затухает у края, за которым есть ещё кнопки.
- 2026-09-10: Рассчитанное число отмечено подписью и чуть более мягким цветом цифр (`color-mix` 85/15) вместо жёлтого подчёркивания — оно дублировало подпись и читалось как ссылка.
- 2026-09-10: Шапка — «план забега» словами («Марафон за 3:44:20 — это 5:19 на километр»), следует за каждым изменением; выбрана из трёх вариантов (нагрудный номер, дорожная разметка). До готовности WASM показывает только «дистанция за время», чтобы при открытии ссылки не мелькал чужой план.
- 2026-09-11: Документация приведена к текущему состоянию: README (возможности, устройство проекта, бот, Mini App, запуск только страницы, CI), AGENTS.md (все пакеты, команды, проверки), сводка и ворота качества в PLAN.md.
- 2026-09-11: Локализация на русский и английский. Страница: словарь `messages` с одинаковыми ключами, разметка помечена `data-i18n`/`data-i18n-label`, числа и даты по локали; переключатель в шапке называет другой язык на нём самом и запоминается в `localStorage`; порядок выбора — ручной выбор, `language_code` из `tgWebAppData`, `navigator.languages`, иначе английский; технические ошибки WASM больше не показываются пользователю. Бот: ответы по `language_code` отправителя (`pkg/telegram/messages.go`), покрытие `pkg/telegram` 65.9% -> 73.5%. Проверено в headless Chrome (36 проверок): в английском режиме нет кириллицы ни в тексте, ни в `aria-label`; смена языка на лету переводит ошибку, шапку, раскладку и сохранённые расчёты; выбор переживает перезагрузку и важнее Telegram; 390 px без горизонтального скролла, консоль чистая.
- 2026-09-11: Docker-образ не собирался: `tinygo/tinygo:0.42.0` работает не под root, и стадия wasm падала на `mkdir /out: Permission denied`. CI этого не ловил — он не запускается для веток. Стадия переключена на `USER root`; образ собран локально (39,2 МБ), бинарь стартует.
- 2026-09-11: PWA: `manifest.json`, иконка «осевая разметка» (`icon.svg` и PNG 192/512/180), статичные `og:`-теги, service worker `sw.js` — сначала сеть, без сети кэш; кэшируются только страница и её файлы. `assets_test.go` отдаёт через настоящий обработчик всё, на что ссылается страница, и проверяет `Content-Type`. Проверено в headless Chrome (11 проверок): манифест без ошибок и устанавливаемый, иконки грузятся, worker активен и управляет страницей; с остановленным сервером ссылка с другим расчётом открывается из кэша и считает (марафон за 3:44:20 — 5:19), консоль чистая.
- 2026-09-11: `json.wasm` из Docker (TinyGo поверх Go образа) на 5,6 КБ больше закоммиченного (TinyGo поверх локального Go 1.26.4); `wasm_exec.js` совпадает побайтно. Обе проверки страницы на докерном `json.wasm` проходят (36/36 и 11/11) — поведение то же, отличаются байты.
- 2026-09-11: Превью ссылки с планом: `GET /` отдаёт `index.html`, где блок между `<!-- og:start -->` и `<!-- og:end -->` заменён тегами с планом из `?d=&t=` (формулировка как в шапке, темп считает тот же движок), абсолютными `og:url` и `og:image` от `HOST` и `og:locale`. Язык превью — `l` из скопированной ссылки (страница получателя его не читает), затем `Accept-Language`, иначе русский. Теги собираются только из разобранных чисел; неполный или неверный план оставляет «Pacer»; страница без маркеров отдаётся без изменений. Покрыто тестами `pkg/http/rest/preview_test.go` и тестом на настоящей странице в `assets_test.go`; в headless Chrome — 39 проверок языка, включая `l` в копируемой ссылке.
- 2026-09-11: Ошибки ввода — под своей группой полей (дистанция, время, темп) вместо одной строки внизу, с точным текстом («Минуты — от 0 до 59», «Темп не может быть 0:00»); поля ссылаются на сообщение через `aria-describedby` и получают `aria-invalid`. Нулевое время теперь отклоняется, как и нулевой темп (раньше молча давало темп 0:00). Проверено в headless Chrome (17 проверок): сообщение появляется только у своей группы, отмечается только неверное поле, исправление снимает ошибку, на десктопе сообщение не сдвигает колонку темпа, 390 px без горизонтального скролла.
