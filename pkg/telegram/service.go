package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"gorun/pkg/calculator"
)

const TgApiURL = "https://api.telegram.org"

const TgMethodSetWebhook = "setWebhook"
const TgMethodDeleteWebhook = "deleteWebhook"

type Service struct {
	bot        *tgbotapi.BotAPI
	calculator *calculator.Service

	startC   chan tgbotapi.Update
	timeC    chan tgbotapi.Update
	paceC    chan tgbotapi.Update
	unknownC chan tgbotapi.Update
	messages chan tgbotapi.Chattable
	done     chan struct{}

	polling  bool
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewService(debugMode bool, host string, token string, calculator *calculator.Service) (*Service, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot api client: %w", err)
	}

	bot.Debug = debugMode

	s := &Service{
		bot:        bot,
		calculator: calculator,
		startC:     make(chan tgbotapi.Update, 64),
		timeC:      make(chan tgbotapi.Update, 64),
		paceC:      make(chan tgbotapi.Update, 64),
		unknownC:   make(chan tgbotapi.Update, 64),
		messages:   make(chan tgbotapi.Chattable, 128),
		done:       make(chan struct{}),
	}

	slog.Info("telegram bot authorized", "username", bot.Self.UserName, "debug_mode", debugMode)

	s.startDispatcher()
	s.startSender()

	httpClient := &http.Client{Timeout: 10 * time.Second}

	if debugMode {
		deleteWebhookURL := fmt.Sprintf("%s/bot%s/%s?drop_pending_updates=true", TgApiURL, token, TgMethodDeleteWebhook)
		if err := callTelegramAPI(httpClient, deleteWebhookURL); err != nil {
			return nil, fmt.Errorf("delete webhook in debug mode: %w", err)
		}

		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60

		updates, err := bot.GetUpdatesChan(u)
		if err != nil {
			return nil, fmt.Errorf("start telegram polling updates: %w", err)
		}

		s.polling = true
		s.startPolling(updates)
		slog.Info("telegram polling started")
	} else {
		webhookURL, err := buildWebhookURL(host, token)
		if err != nil {
			return nil, fmt.Errorf("build webhook url: %w", err)
		}

		setWebhookURL := fmt.Sprintf(
			"%s/bot%s/%s?%s",
			TgApiURL,
			token,
			TgMethodSetWebhook,
			url.Values{"url": []string{webhookURL}}.Encode(),
		)

		if err := callTelegramAPI(httpClient, setWebhookURL); err != nil {
			return nil, fmt.Errorf("set webhook: %w", err)
		}

		slog.Info("telegram webhook configured", "host", host)
	}

	return s, nil
}

func (s *Service) DoUpdate(update tgbotapi.Update) {
	s.handleUpdate(update)
}

func (s *Service) Close(ctx context.Context) error {
	s.stopOnce.Do(func() {
		close(s.done)
		if s.polling {
			s.bot.StopReceivingUpdates()
		}
	})

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		s.wg.Wait()
	}()

	select {
	case <-finished:
		slog.Info("telegram service stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) handleUpdate(update tgbotapi.Update) {
	if update.Message != nil && update.Message.IsCommand() {
		command := update.Message.Command()
		switch command {
		case "start":
			s.enqueueUpdate(s.startC, command, update)
		case "time":
			s.enqueueUpdate(s.timeC, command, update)
		case "pace":
			s.enqueueUpdate(s.paceC, command, update)
		default:
			s.enqueueUpdate(s.unknownC, command, update)
		}
		return
	}

	if update.Message != nil {
		s.enqueueUpdate(s.startC, "message", update)
		return
	}

	slog.Debug("telegram update ignored: message is nil")
}

func (s *Service) enqueueUpdate(ch chan tgbotapi.Update, source string, update tgbotapi.Update) {
	select {
	case <-s.done:
		return
	case ch <- update:
	default:
		slog.Warn("telegram update dropped: queue is full", "source", source)
	}
}

func (s *Service) enqueueMessage(message tgbotapi.Chattable) {
	select {
	case <-s.done:
		return
	case s.messages <- message:
	default:
		slog.Warn("telegram outgoing message dropped: queue is full")
	}
}

func (s *Service) startDispatcher() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		for {
			select {
			case <-s.done:
				return
			case u := <-s.startC:
				s.enqueueMessage(handleStartCmd(&u))
			case u := <-s.timeC:
				s.enqueueMessage(s.handleTimeCmd(&u))
			case u := <-s.paceC:
				s.enqueueMessage(s.handlePaceCmd(&u))
			case u := <-s.unknownC:
				s.enqueueMessage(handleUnknownCmd(&u))
			}
		}
	}()
}

func (s *Service) startSender() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		for {
			select {
			case <-s.done:
				return
			case msg := <-s.messages:
				if _, err := s.bot.Send(msg); err != nil {
					slog.Error("send telegram message failed", "err", err)
				}
			}
		}
	}()
}

func (s *Service) startPolling(updates tgbotapi.UpdatesChannel) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		for {
			select {
			case <-s.done:
				return
			case update, ok := <-updates:
				if !ok {
					return
				}
				s.handleUpdate(update)
			}
		}
	}()
}

func callTelegramAPI(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func buildWebhookURL(host, token string) (string, error) {
	normalizedHost := strings.TrimSpace(host)
	if normalizedHost == "" {
		return "", errors.New("host is empty")
	}

	if !strings.Contains(normalizedHost, "://") {
		normalizedHost = "https://" + normalizedHost
	}

	webhookURL, err := url.Parse(normalizedHost)
	if err != nil {
		return "", err
	}
	if webhookURL.Scheme == "" || webhookURL.Host == "" {
		return "", fmt.Errorf("invalid host %q", host)
	}
	if webhookURL.Scheme != "https" {
		return "", fmt.Errorf("webhook host must use https scheme, got %q", webhookURL.Scheme)
	}

	basePath := strings.TrimRight(webhookURL.Path, "/")
	if basePath == "" {
		webhookURL.Path = "/" + token
	} else {
		webhookURL.Path = basePath + "/" + token
	}

	webhookURL.RawQuery = ""
	webhookURL.Fragment = ""

	return webhookURL.String(), nil
}

func handleStartCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	buf := bytes.NewBufferString("Calculate Your Running Pace")

	buf.WriteString("\n")
	buf.WriteString("Please, enter of the following commands:\n\n")
	buf.WriteString(
		"[/time]() - calculate time, e.g. */time 4m50s 21095*, where first - pace, second - distance\n",
	)
	buf.WriteString(
		"[/pace]() - calculate pace, e.g. */pace 21097 1h38m48s*, where first - distance, second - time\n",
	)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, buf.String())
	msg.ParseMode = "markdown"
	return msg
}

func handleUnknownCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	return buildMsg(update, fmt.Sprintf(
		"Unknown command: /%s\n\nAvailable commands: /start, /time, /pace",
		update.Message.Command(),
	))
}

func (s *Service) handleTimeCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	arguments, err := extractArguments(update)
	if err != nil {
		return buildMsg(update, err.Error())
	}

	if len(arguments) != 2 {
		return buildMsg(update, "should be 2 arguments: (pace, dist) separated by a space")
	}

	paceDuration, err := time.ParseDuration(arguments[0])
	if err != nil {
		return buildMsg(update, "invalid pace value: "+arguments[0])
	}

	dist, err := strconv.Atoi(arguments[1])
	if err != nil {
		return buildMsg(update, "invalid dist value: "+arguments[1])
	}

	resultTime := s.calculator.Time(dist, paceDuration)
	return buildMsg(update, resultTime.String())
}

func (s *Service) handlePaceCmd(update *tgbotapi.Update) tgbotapi.Chattable {
	arguments, err := extractArguments(update)
	if err != nil {
		return buildMsg(update, err.Error())
	}

	if len(arguments) != 2 {
		return buildMsg(update, "should be 2 arguments: (pace, dist) separated by a space")
	}

	dist, err := strconv.Atoi(arguments[0])
	if err != nil {
		return buildMsg(update, "invalid dist value: "+arguments[0])
	}

	timeDuration, err := time.ParseDuration(arguments[1])
	if err != nil {
		return buildMsg(update, "invalid time value: "+arguments[1])
	}

	resultPace := s.calculator.Pace(dist, timeDuration)
	return buildMsg(update, resultPace.String())
}

func extractArguments(update *tgbotapi.Update) ([]string, error) {
	arguments := update.Message.CommandArguments()
	if arguments == "" {
		return nil, errors.New("empty arguments")
	}

	return strings.Fields(arguments), nil
}

func buildMsg(update *tgbotapi.Update, text string) tgbotapi.Chattable {
	return tgbotapi.NewMessage(update.Message.Chat.ID, text)
}
