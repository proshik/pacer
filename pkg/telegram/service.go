package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/calculator"
	"gorun/pkg/card"
)

// Queue sizes. Enqueues are non blocking: when a queue is full the update is
// dropped with a warning rather than stalling the webhook handler, which must
// answer Telegram promptly.
const (
	updateQueueSize  = 64
	messageQueueSize = 128
)

type Service struct {
	bot        *bot.Bot
	calculator calculator.Engine

	startC   chan *models.Update
	timeC    chan *models.Update
	paceC    chan *models.Update
	cardC    chan *models.Update
	unknownC chan *models.Update
	messages chan outgoing
	done     chan struct{}

	polling       bool
	cancelPolling context.CancelFunc
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// newService builds the queues without touching the network, so tests can
// exercise routing and handlers without a bot client.
func newService(engine calculator.Engine) *Service {
	return &Service{
		calculator: engine,
		startC:     make(chan *models.Update, updateQueueSize),
		timeC:      make(chan *models.Update, updateQueueSize),
		paceC:      make(chan *models.Update, updateQueueSize),
		cardC:      make(chan *models.Update, updateQueueSize),
		unknownC:   make(chan *models.Update, updateQueueSize),
		messages:   make(chan outgoing, messageQueueSize),
		done:       make(chan struct{}),
	}
}

// NewService connects to Telegram and starts the worker goroutines.
//
// In debug mode any webhook is removed and updates are long polled; otherwise
// a webhook pointing at host is registered.
func NewService(ctx context.Context, debugMode bool, host string, token string, engine calculator.Engine) (*Service, error) {
	s := newService(engine)

	client, err := bot.New(token, bot.WithDefaultHandler(
		func(_ context.Context, _ *bot.Bot, update *models.Update) {
			s.handleUpdate(update)
		},
	))
	if err != nil {
		return nil, fmt.Errorf("create telegram bot api client: %w", err)
	}

	s.bot = client
	s.startDispatcher()
	s.startSender()

	// The menu is a nicety: failing to publish it must not keep the bot down.
	if err := publishCommands(ctx, client); err != nil {
		slog.Warn("publish command menu failed", "err", err)
	}

	if debugMode {
		if _, err := client.DeleteWebhook(ctx, &bot.DeleteWebhookParams{DropPendingUpdates: true}); err != nil {
			return nil, fmt.Errorf("delete webhook in debug mode: %w", err)
		}

		pollCtx, cancel := context.WithCancel(context.Background())
		s.cancelPolling = cancel
		s.polling = true

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			client.Start(pollCtx)
		}()

		slog.Info("telegram polling started")

		return s, nil
	}

	webhookURL, err := buildWebhookURL(host, token)
	if err != nil {
		return nil, fmt.Errorf("build webhook url: %w", err)
	}

	if _, err := client.SetWebhook(ctx, &bot.SetWebhookParams{URL: webhookURL}); err != nil {
		return nil, fmt.Errorf("set webhook: %w", err)
	}

	slog.Info("telegram webhook configured", "host", host)

	return s, nil
}

// DoUpdate feeds an update that arrived over the webhook.
func (s *Service) DoUpdate(update *models.Update) {
	s.handleUpdate(update)
}

func (s *Service) Close(ctx context.Context) error {
	s.stopOnce.Do(func() {
		close(s.done)
		if s.cancelPolling != nil {
			s.cancelPolling()
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

// parseCommand extracts a bot command and its arguments from a message.
//
// go-telegram/bot ships no equivalent of the old IsCommand/Command helpers, so
// the bot_command entity Telegram places at offset 0 is decoded here. Offsets
// are UTF-16 units, which matches byte offsets for the ASCII command token.
func parseCommand(message *models.Message) (command string, arguments string) {
	if message == nil || len(message.Entities) == 0 {
		return "", ""
	}

	entity := message.Entities[0]
	if entity.Type != models.MessageEntityTypeBotCommand || entity.Offset != 0 {
		return "", ""
	}

	if entity.Length < 1 || entity.Length > len(message.Text) {
		return "", ""
	}

	command = message.Text[1:entity.Length]
	if at := strings.Index(command, "@"); at != -1 {
		command = command[:at]
	}

	if len(message.Text) > entity.Length {
		arguments = message.Text[entity.Length+1:]
	}

	return command, arguments
}

func (s *Service) handleUpdate(update *models.Update) {
	if update == nil || update.Message == nil {
		slog.Debug("telegram update ignored: message is nil")
		return
	}

	command, _ := parseCommand(update.Message)
	switch command {
	case "":
		s.enqueueUpdate(s.startC, "message", update)
	case "start":
		s.enqueueUpdate(s.startC, command, update)
	case "time":
		s.enqueueUpdate(s.timeC, command, update)
	case "pace":
		s.enqueueUpdate(s.paceC, command, update)
	case "card":
		s.enqueueUpdate(s.cardC, command, update)
	default:
		s.enqueueUpdate(s.unknownC, command, update)
	}
}

func (s *Service) enqueueUpdate(ch chan *models.Update, source string, update *models.Update) {
	select {
	case <-s.done:
		return
	case ch <- update:
	default:
		slog.Warn("telegram update dropped: queue is full", "source", source)
	}
}

func (s *Service) enqueueMessage(reply outgoing) {
	select {
	case <-s.done:
		return
	case s.messages <- reply:
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
				s.enqueueMessage(handleStartCmd(u))
			case u := <-s.timeC:
				s.enqueueMessage(s.handleTimeCmd(u))
			case u := <-s.paceC:
				s.enqueueMessage(s.handlePaceCmd(u))
			case u := <-s.cardC:
				s.enqueueMessage(s.handleCardCmd(u))
			case u := <-s.unknownC:
				s.enqueueMessage(handleUnknownCmd(u))
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
			case reply := <-s.messages:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := reply.send(ctx, s.bot); err != nil {
					slog.Error("send telegram reply failed", "err", err)
				}
				cancel()
			}
		}
	}()
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

// outgoing is a reply on its way to Telegram: words or a picture. The sender
// goroutine does not care which.
type outgoing interface {
	send(ctx context.Context, client *bot.Bot) error
}

type textReply struct {
	params *bot.SendMessageParams
}

func (r textReply) send(ctx context.Context, client *bot.Bot) error {
	_, err := client.SendMessage(ctx, r.params)

	return err
}

type photoReply struct {
	params *bot.SendPhotoParams
}

func (r photoReply) send(ctx context.Context, client *bot.Bot) error {
	_, err := client.SendPhoto(ctx, r.params)

	return err
}

func handleStartCmd(update *models.Update) outgoing {
	return buildMsg(update, textsFor(update.Message).greeting)
}

func handleUnknownCmd(update *models.Update) outgoing {
	command, _ := parseCommand(update.Message)

	return buildMsg(update, fmt.Sprintf(textsFor(update.Message).unknownCommand, command))
}

func (s *Service) handleTimeCmd(update *models.Update) outgoing {
	t := textsFor(update.Message)

	arguments := extractArguments(update)
	if len(arguments) == 0 {
		return buildMsg(update, t.emptyArguments)
	}

	if len(arguments) != 2 {
		return buildMsg(update, t.timeArgCount)
	}

	paceDuration, err := time.ParseDuration(arguments[0])
	if err != nil {
		return buildMsg(update, t.invalidPace+arguments[0])
	}

	dist, err := strconv.Atoi(arguments[1])
	if err != nil {
		return buildMsg(update, t.invalidDist+arguments[1])
	}

	return buildMsg(update, s.calculator.Time(dist, paceDuration).String())
}

func (s *Service) handlePaceCmd(update *models.Update) outgoing {
	t := textsFor(update.Message)

	arguments := extractArguments(update)
	if len(arguments) == 0 {
		return buildMsg(update, t.emptyArguments)
	}

	if len(arguments) != 2 {
		return buildMsg(update, t.paceArgCount)
	}

	dist, err := strconv.Atoi(arguments[0])
	if err != nil {
		return buildMsg(update, t.invalidDist+arguments[0])
	}

	timeDuration, err := time.ParseDuration(arguments[1])
	if err != nil {
		return buildMsg(update, t.invalidTime+arguments[1])
	}

	return buildMsg(update, s.calculator.Pace(dist, timeDuration).String())
}

// handleCardCmd draws the plan as a picture, which forwards into a chat as one
// message and shows the splits a line of text cannot.
func (s *Service) handleCardCmd(update *models.Update) outgoing {
	t := textsFor(update.Message)

	arguments := extractArguments(update)
	if len(arguments) == 0 {
		return buildMsg(update, t.emptyArguments)
	}

	if len(arguments) != 2 {
		return buildMsg(update, t.paceArgCount)
	}

	dist, err := strconv.Atoi(arguments[0])
	if err != nil {
		return buildMsg(update, t.invalidDist+arguments[0])
	}

	raceTime, err := time.ParseDuration(arguments[1])
	if err != nil {
		return buildMsg(update, t.invalidTime+arguments[1])
	}

	plan := card.Plan{Distance: dist, Time: raceTime, Language: t.language}
	picture, err := card.Render(plan)
	if err != nil {
		slog.Warn("draw card failed", "err", err, "dist", dist, "time", raceTime)
		return buildMsg(update, t.cardFailed)
	}

	return photoReply{params: &bot.SendPhotoParams{
		ChatID:  update.Message.Chat.ID,
		Photo:   &models.InputFileUpload{Filename: "pacer.png", Data: bytes.NewReader(picture)},
		Caption: card.Caption(plan),
	}}
}

func extractArguments(update *models.Update) []string {
	_, arguments := parseCommand(update.Message)

	return strings.Fields(arguments)
}

func buildMsg(update *models.Update, text string) outgoing {
	return textReply{params: &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   text,
	}}
}
