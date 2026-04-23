# gorun

Калькулятор бегового темпа/времени с:
- backend на Go (`main.go`);
- web-интерфейсом из `assets/`;
- WASM-модулем (`assets/json.wasm`), который собирается отдельно из `cmd/wasm`.

## Почему запуск устроен по-разному

В проекте есть два разных executable:

1. `main.go` (корень проекта)  
   Это основной сервер: HTTP-роуты, Telegram-интеграция, раздача `assets`.

2. `cmd/wasm/main.go`  
   Это код для браузера (WASM), собирается только с `GOOS=js GOARCH=wasm`.

Поэтому:
- локально для запуска сервера достаточно `go run .`;
- для обновления фронтенд-логики в WASM нужно отдельно выполнить `./build.sh`;
- в production обычно заранее собирают/коммитят `assets/json.wasm` и `assets/wasm_exec.js`, а сервер просто отдает эти файлы.

## Требования

- Go 1.26+
- Docker (опционально)

## Переменные окружения

Обязательные:
- `PORT`
- `TELEGRAM_TOKEN`
- `HOST`

Опциональная:
- `DEBUG` (`true/false`, по умолчанию `false`)
- `LOG_LEVEL` (`debug|info|warn|error`; если не задан, уровень берется из `DEBUG`)

### Приоритет источников конфигурации

При старте `main.go`:
1. Сначала читаются уже заданные переменные окружения (IDE/OS/CI).
2. Если в корне есть файл `.env`, из него подхватываются только отсутствующие переменные.

Это значит, что значения из окружения имеют приоритет над `.env`.

## Локальный запуск в IDE (рекомендуемый)

1. Создайте `.env` в корне проекта:

```bash
cp .env.example .env
```

2. Заполните `.env` реальными значениями (`TELEGRAM_TOKEN` и т.д.).
3. Запускайте `main.go`/`go run .` из IDE без ручного добавления env в Run Configuration.

## Локальный запуск из терминала

Вариант 1: через `.env` (как выше), просто:

```bash
go run .
```

Вариант 2: явно через переменные:

```bash
PORT=8080 TELEGRAM_TOKEN=token HOST=http://localhost:8080 DEBUG=true go run .
```

## Как собирается WASM

Скрипт `build.sh` делает две вещи:
1. Копирует `wasm_exec.js` из текущего Go SDK в `assets/`.
2. Собирает `cmd/wasm` в `assets/json.wasm` с `GOOS=js GOARCH=wasm`.

Запуск:

```bash
./build.sh
```

Ручная эквивалентная команда:

```bash
GOOS=js GOARCH=wasm go build -o assets/json.wasm ./cmd/wasm
```

Примечание: пакет `cmd/wasm` имеет build-tags `js/wasm`, поэтому обычный `go run ./cmd/wasm` не запускает WASM-версию. Для удобства в не-WASM окружении есть fallback `main_nowasm.go` с подсказкой.

## Рекомендуемый цикл разработки

1. Изменили backend (`main.go`, `pkg/...`) -> `go run .`
2. Изменили WASM-логику (`cmd/wasm/...`) -> `./build.sh`, затем перезапуск сервера/обновление страницы.
3. Перед коммитом:

```bash
go test ./ ./pkg/...
go vet ./ ./pkg/...
go build ./ ./pkg/...
./build.sh
```

## Production / Docker

Текущий `Dockerfile` собирает только backend binary (`/pacer`) и запускает его.
WASM-файлы должны уже лежать в `assets/` в актуальном состоянии к моменту `docker build`.

Сборка и запуск контейнера:

```bash
docker build -t gorun .
docker run --rm -p 8080:80 \
  -e PORT=80 \
  -e TELEGRAM_TOKEN=token \
  -e HOST=http://localhost:8080 \
  -e DEBUG=true \
  gorun
```

## Типичные проблемы

1. `PORT must be set`  
   Нет `PORT` ни в окружении, ни в `.env`.

2. `TELEGRAM_TOKEN must be set` / `HOST must be set`  
   Аналогично, проверьте `.env` и Run Configuration.

3. IDE подсвечивает `syscall/js`  
   Это нормально для не-WASM таргета. Файл `cmd/wasm/main.go` ограничен build-tags `js && wasm`.
