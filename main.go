package main

import (
	"cmp"
	"context"
	"embed"
	"errors"
	"fmt"
	"gorun/pkg/calculator"
	"gorun/pkg/history"
	"gorun/pkg/http/rest"
	"gorun/pkg/telegram"
	"gorun/pkg/wasmcalc"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// 1. забираю из переменных значения порта, токена, хоста, признак локальная тачка или нет.
// 2. если локальная тачка, то создаю бота через polling, иначе webhook.

// 0. по командам, или кнопкам (снизу): тайм, пейс сделать возможность посчитать по заданнм после параметрам.
// Отправляет команду, зате подсказки: ввведи расстояние в киломлетрах или с точностью до 100м через точку или запятую,
// введи пейс через пробел или двоеточие, где группа цифр это часы, минуты, секунды. Если групп
// цифр всего 2, тогда это минуты и часы, если одна то это секунды. например: 5 15, 03:35:00, 03:52, 1 39 00
// 1. делаю клавиатуру, для возмности задания
//- (дистанция + пейс) = тайм
//- (дистанция + тайм) = пейс
// 2. используя библиотеку и по отдельной кнопке генерирую картинку (svg) с рерзультатом по заданным параметрам

//go:embed assets
var assets embed.FS

func main() {
	if err := loadDotEnvIfExists(".env"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to load .env: %v\n", err)
		os.Exit(1)
	}

	debug, debugParseErr := parseDebugMode(os.Getenv("DEBUG"))
	logger, effectiveLevel, logLevelParseErr := newLogger(os.Getenv("LOG_LEVEL"), debug)
	slog.SetDefault(logger)

	if debugParseErr != nil {
		slog.Warn("invalid DEBUG value, fallback to false", "value", os.Getenv("DEBUG"))
	}

	if logLevelParseErr != nil {
		slog.Warn("invalid LOG_LEVEL value, fallback to default", "value", os.Getenv("LOG_LEVEL"), "fallback_level", effectiveLevel.String())
	}

	port, err := requiredEnv("PORT")
	if err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}

	tgToken, err := requiredEnv("TELEGRAM_TOKEN")
	if err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}

	host, err := requiredEnv("HOST")
	if err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}

	engineName := os.Getenv("CALC_ENGINE")
	c, closeEngine, err := newCalculationEngine(engineName)
	if err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
	slog.Info("calculation engine selected", "engine", cmp.Or(strings.ToLower(strings.TrimSpace(engineName)), "native"))

	t, err := telegram.NewService(context.Background(), debug, host, tgToken, c)
	if err != nil {
		slog.Error("telegram service init failed", "err", err)
		os.Exit(1)
	}

	store, err := openHistory(os.Getenv("DB_PATH"))
	if err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
	slog.Info("saved runs history", "enabled", store != nil)

	handler, err := rest.NewHandler(debug, tgToken, t, c, store, assets)
	if err != nil {
		slog.Error("http handler init failed", "err", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf(":%s", port)
	slog.Info("server starting", "addr", addr, "debug", debug)

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	shutdownSignals, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped unexpectedly", "err", err)
			os.Exit(1)
		}
		slog.Info("server stopped")
		return
	case <-shutdownSignals.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown failed", "err", err)
	} else {
		slog.Info("http server stopped")
	}

	if err := t.Close(shutdownCtx); err != nil {
		slog.Error("telegram shutdown failed", "err", err)
	} else {
		slog.Info("telegram service closed")
	}

	if err := closeEngine(shutdownCtx); err != nil {
		slog.Error("calculation engine shutdown failed", "err", err)
	}

	if store != nil {
		if err := store.Close(); err != nil {
			slog.Error("history database close failed", "err", err)
		}
	}

	if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server returned error after shutdown", "err", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
}

// newCalculationEngine picks how the formulas are executed. "native" links
// them in directly; "wasm" runs the very same artifact the browser loads,
// through wazero, so server and client cannot drift apart.
//
// The returned close function is always safe to call.
func newCalculationEngine(value string) (calculator.Engine, func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "native":
		return calculator.NewService(), noop, nil
	case "wasm":
		engine, err := wasmcalc.New(context.Background())
		if err != nil {
			return nil, noop, fmt.Errorf("create wasm calculation engine: %w", err)
		}

		return engine, engine.Close, nil
	default:
		return nil, noop, fmt.Errorf("unknown CALC_ENGINE %q, want \"native\" or \"wasm\"", value)
	}
}

// openHistory opens the saved-runs database when DB_PATH is set. Without it the
// calculator works as before and only the history endpoints answer 503.
func openHistory(path string) (*history.Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}

	store, err := history.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open history database %q: %w", path, err)
	}

	return store, nil
}

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s must be set", key)
	}

	return value, nil
}

func parseDebugMode(debugValue string) (bool, error) {
	if debugValue == "" {
		return false, nil
	}

	debugMode, err := strconv.ParseBool(debugValue)
	if err != nil {
		return false, err
	}

	return debugMode, nil
}

func newLogger(logLevelValue string, debug bool) (*slog.Logger, slog.Level, error) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	var parseErr error
	if strings.TrimSpace(logLevelValue) != "" {
		switch strings.ToLower(strings.TrimSpace(logLevelValue)) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			parseErr = fmt.Errorf("unknown LOG_LEVEL: %q", logLevelValue)
		}
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler), level, parseErr
}

func loadDotEnvIfExists(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid .env line %d: %q", i+1, line)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("empty key in .env line %d", i+1)
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
			}
			if value[0] == '\'' && value[len(value)-1] == '\'' {
				value = value[1 : len(value)-1]
			}
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s from .env line %d: %w", key, i+1, err)
		}
	}

	return nil
}
