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
- TinyGo 0.42+ — собирает браузерный бандл (`brew tap tinygo-org/tools`,
  `brew trust --formula tinygo-org/tools/tinygo`, `brew install tinygo`)
- Docker (опционально)

## Переменные окружения

Обязательные:
- `PORT`
- `TELEGRAM_TOKEN`
- `HOST`

Опциональные:
- `DEBUG` (`true/false`, по умолчанию `false`)
- `LOG_LEVEL` (`debug|info|warn|error`; если не задан, уровень берется из `DEBUG`)
- `CALC_ENGINE` (`native` по умолчанию, либо `wasm`) — чем считать темп и время
- `DB_PATH` — файл SQLite для расчётов, сохранённых в Mini App. Без него история отключена:
  `/api/v1/runs` отвечает `503`, остальное работает. В Docker по умолчанию `/data/pacer.db`

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

## HTTP API

Доступен в обоих режимах (`DEBUG` на него не влияет):

```bash
curl "http://localhost:8080/api/v1/time?dist=21097&pace=4m50s"
curl "http://localhost:8080/api/v1/pace?dist=21097&time=1h38m48s"
curl "http://localhost:8080/healthz"
```

Ответ:

```json
{"result":"1h41m57s","total_seconds":6117,"hours":1,"minutes":41,"seconds":57}
```

При ошибке валидации возвращается `400` и `{"error":"validation failed","details":{...}}`.

### Анализ забега

```bash
# сплиты при ровном темпе; step — шаг в метрах, по умолчанию 1000, не больше 500 отметок
curl "http://localhost:8080/api/v1/splits?dist=21097&pace=4m30s&step=5000"
# прогноз на 1, 3, 5, 10 км, полумарафон и марафон по Ригелю и Кэмерону
curl "http://localhost:8080/api/v1/predict?dist=5000&time=20m0s"
# VDOT по Дэниелсу, тренировочные темпы (на км) и эквивалентные результаты
curl "http://localhost:8080/api/v1/vdot?dist=5000&time=19m57s"
```

Длительности в этих ответах — объект `{"text":"50m0s","seconds":3000}`.

Формулы: Ригель — T₂ = T₁·(D₂/D₁)^1.06; Кэмерон — модель для дистанций от 800 м до марафона;
VDOT — уравнения Дэниелса–Гилберта. Зоны: E — 59–74% VDOT, T — 88%, I — 98%, M — темп
эквивалентного марафона. Темп повторов (R) не считается: источники не задают его процентом.

Эти три эндпоинта считают нативно — `CALC_ENGINE=wasm` на них пока не действует.

### Mini App: пользователь и история

Запросы подписываются данными запуска Telegram — заголовок `Authorization: tma <initData>`.
Данные принимаются 24 часа с момента открытия приложения.

```bash
curl -H "Authorization: tma $INIT_DATA" http://localhost:8080/api/v1/me
curl -H "Authorization: tma $INIT_DATA" http://localhost:8080/api/v1/runs
curl -X POST -H "Authorization: tma $INIT_DATA" \
  -d '{"distance":21097,"time_seconds":5928}' http://localhost:8080/api/v1/runs
curl -X DELETE -H "Authorization: tma $INIT_DATA" http://localhost:8080/api/v1/runs/1
```

Хранятся дистанция и время — темп из них выводится. Чужую запись удалить нельзя: ответ `404`.

## Как собирается WASM

Скрипт `build.sh` делает три вещи:
1. Копирует в `assets/` файл `wasm_exec.js` того же тулчейна, которым собирается бандл: у TinyGo
   и обычного Go эти файлы разные и не взаимозаменяемы.
2. Собирает `cmd/wasm` в `assets/json.wasm` — код для браузера. По умолчанию на TinyGo: бандл
   весит 0,93 МБ вместо 4,54 МБ на Go 1.27 (257 КБ вместо 917 КБ после brotli), а ответы
   функций совпадают побайтно. `WASM_COMPILER=go ./build.sh` соберёт обычным Go.
3. Собирает `cmd/calcwasm` в `pkg/wasmcalc/calc.wasm` (`GOOS=wasip1 GOARCH=wasm`,
   `-buildmode=c-shared`) — WASI-reactor для сервера.

Запуск:

```bash
./build.sh
```

Ручные эквивалентные команды:

```bash
tinygo build -target=wasm -no-debug -o assets/json.wasm ./cmd/wasm
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o pkg/wasmcalc/calc.wasm ./cmd/calcwasm
```

Примечание: у обоих wasm-пакетов есть build-tags, поэтому обычный `go run ./cmd/wasm` не
запускает WASM-версию. Для удобства в не-WASM окружении рядом лежит fallback `main_nowasm.go`
с подсказкой.

## Один артефакт в двух хостах

`pkg/calculator` — единственное место, где живут формулы. Браузер исполняет их как WASM, и
сервер умеет делать то же самое: при `CALC_ENGINE=wasm` он через wazero запускает
`pkg/wasmcalc/calc.wasm`, собранный из того же пакета.

Смысл — исключить расхождение между ботом, API и страницей: это один скомпилированный
артефакт, а не две копии кода на разных языках. Эквивалентность нативного и wasm-движка
проверяется тестом в `pkg/wasmcalc`.

По умолчанию используется `native` — wazero не нужен для обычной работы, он включается
осознанно.

## Рекомендуемый цикл разработки

1. Изменили backend (`main.go`, `pkg/...`) -> `go run .`
2. Изменили WASM-логику (`cmd/wasm/...`, `cmd/calcwasm/...`, `pkg/calculator/...`) ->
   `./build.sh`, затем перезапуск сервера/обновление страницы.
3. Перед коммитом:

```bash
go test -race ./ ./pkg/...
go vet ./ ./pkg/...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.0 run ./...
go build ./ ./pkg/...
./build.sh
```

## Production / Docker

`Dockerfile` многоступенчатый: браузерный бандл собирается в стадии `tinygo/tinygo:0.42.0`,
серверный reactor — обычным Go, затем backend binary (`/pacer`), который их встраивает.

Сборка и запуск контейнера:

```bash
docker build -t gorun .
docker run --rm -p 8080:80 \
  -e PORT=80 \
  -e TELEGRAM_TOKEN=token \
  -e HOST=http://localhost:8080 \
  -e DEBUG=true \
  -v pacer-data:/data \
  gorun
```

## Типичные проблемы

1. `PORT must be set`  
   Нет `PORT` ни в окружении, ни в `.env`.

2. `TELEGRAM_TOKEN must be set` / `HOST must be set`  
   Аналогично, проверьте `.env` и Run Configuration.

3. IDE подсвечивает `syscall/js`  
   Это нормально для не-WASM таргета. Файл `cmd/wasm/main.go` ограничен build-tags `js && wasm`.
