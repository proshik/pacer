package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/metrics"
	"gorun/pkg/plan"
)

// Queue sizes. Enqueues are non blocking: when a queue is full the update is
// dropped with a warning rather than stalling the webhook handler, which must
// answer Telegram promptly.
const (
	updateQueueSize  = 64
	messageQueueSize = 128
)

type Service struct {
	bot     *bot.Bot
	siteURL string

	// metrics may be nil: every recording method is a no-op then.
	metrics *metrics.Metrics

	// greetingPhotos remembers the file_id Telegram gives each greeting
	// picture, so it is uploaded once per language rather than on every /start.
	greetingPhotos *photoCache

	startC    chan *models.Update
	textC     chan *models.Update
	timeC     chan *models.Update
	paceC     chan *models.Update
	cardC     chan *models.Update
	unknownC  chan *models.Update
	inlineC   chan *models.Update
	callbackC chan *models.Update
	messages  chan outgoing
	done      chan struct{}

	polling       bool
	cancelPolling context.CancelFunc
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// newService builds the queues without touching the network, so tests can
// exercise routing and handlers without a bot client.
func newService() *Service {
	return newServiceWithMetrics(nil)
}

func newServiceWithMetrics(m *metrics.Metrics) *Service {
	return &Service{
		metrics:   m,
		startC:    make(chan *models.Update, updateQueueSize),
		textC:     make(chan *models.Update, updateQueueSize),
		callbackC: make(chan *models.Update, updateQueueSize),
		inlineC:   make(chan *models.Update, updateQueueSize),
		timeC:     make(chan *models.Update, updateQueueSize),
		paceC:     make(chan *models.Update, updateQueueSize),
		cardC:     make(chan *models.Update, updateQueueSize),
		unknownC:  make(chan *models.Update, updateQueueSize),
		messages:  make(chan outgoing, messageQueueSize),
		done:      make(chan struct{}),

		greetingPhotos: newPhotoCache(),
	}
}

// NewService connects to Telegram and starts the worker goroutines.
//
// In debug mode any webhook is removed and updates are long polled; otherwise
// a webhook pointing at host is registered. m may be nil.
func NewService(ctx context.Context, debugMode bool, host string, token string, m *metrics.Metrics) (*Service, error) {
	s := newServiceWithMetrics(m)
	s.siteURL = webAppSite(host)

	client, err := bot.New(token, bot.WithDefaultHandler(
		func(_ context.Context, _ *bot.Bot, update *models.Update) {
			s.handleUpdate(update)
		},
	), bot.WithAllowedUpdates(bot.AllowedUpdates{"message", "callback_query", "inline_query"}))
	if err != nil {
		return nil, fmt.Errorf("create telegram bot api client: %w", err)
	}

	s.bot = client
	s.startDispatcher()
	s.startSender()

	// The menu and the descriptions are niceties: failing to publish them must
	// not keep the bot down.
	if err := publishCommands(ctx, client); err != nil {
		slog.Warn("publish command menu failed", "err", err)
	}
	if err := publishDescriptions(ctx, client); err != nil {
		slog.Warn("publish bot description failed", "err", err)
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

	if _, err := client.SetWebhook(ctx, &bot.SetWebhookParams{URL: webhookURL, AllowedUpdates: []string{"message", "callback_query", "inline_query"}}); err != nil {
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
	if update == nil {
		return
	}

	if update.InlineQuery != nil {
		s.enqueueUpdate(s.inlineC, "inline", update)
		return
	}

	if update.CallbackQuery != nil {
		s.enqueueUpdate(s.callbackC, "callback", update)
		return
	}

	if update.Message == nil {
		slog.Debug("telegram update ignored: message is nil")
		return
	}

	command, _ := parseCommand(update.Message)
	switch command {
	case "":
		s.enqueueUpdate(s.textC, "message", update)
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
		s.metrics.BotDropped(metrics.QueueUpdates)
		slog.Warn("telegram update dropped: queue is full", "source", source)
	}
}

func (s *Service) enqueueMessage(reply outgoing) {
	select {
	case <-s.done:
		return
	case s.messages <- reply:
	default:
		s.metrics.BotDropped(metrics.QueueOutgoing)
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
				s.metrics.BotUpdate(metrics.UpdateMessage)
				s.enqueueMessage(s.handleStartCmd(u))
			case u := <-s.textC:
				s.metrics.BotUpdate(metrics.UpdateMessage)
				s.enqueueMessage(s.handleText(u))
			case u := <-s.inlineC:
				s.metrics.BotUpdate(metrics.UpdateInlineQuery)
				s.enqueueMessage(s.handleInline(u))
			case u := <-s.callbackC:
				s.metrics.BotUpdate(metrics.UpdateCallbackQuery)
				s.enqueueMessage(s.handleCallback(u))
			case u := <-s.timeC:
				s.metrics.BotUpdate(metrics.UpdateMessage)
				s.enqueueMessage(s.handleTimeCmd(u))
			case u := <-s.paceC:
				s.metrics.BotUpdate(metrics.UpdateMessage)
				s.enqueueMessage(s.handlePaceCmd(u))
			case u := <-s.cardC:
				s.metrics.BotUpdate(metrics.UpdateMessage)
				s.enqueueMessage(s.handleCardCmd(u))
			case u := <-s.unknownC:
				s.metrics.BotUpdate(metrics.UpdateMessage)
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
					s.metrics.BotSendError(replyKind(reply))
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

// outgoing is a reply on its way to Telegram: words, a picture, or the answer
// to a button. The sender
// goroutine does not care which.
type outgoing interface {
	send(ctx context.Context, client *bot.Bot) error
}

// replyKind names a reply for the send error counter.
func replyKind(reply outgoing) string {
	switch reply.(type) {
	case textReply:
		return "text"
	case photoReply:
		return "photo"
	case greetingReply:
		return "greeting"
	case callbackReply:
		return "callback"
	case inlineReply:
		return "inline"
	default:
		return "other"
	}
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

// handleStartCmd greets with the greeting picture, the examples under it and
// a button that opens the app.
func (s *Service) handleStartCmd(update *models.Update) outgoing {
	t := textsFor(update.Message)
	text := s.greetingMessage(update)

	return greetingReply{
		photo: &bot.SendPhotoParams{
			ChatID:      text.ChatID,
			Caption:     text.Text,
			ParseMode:   text.ParseMode,
			ReplyMarkup: text.ReplyMarkup,
		},
		text:     text,
		language: t.language,
		cache:    s.greetingPhotos,
	}
}

// greetingMessage is the greeting in words alone: the answer to a message the
// bot did not understand, and what /start falls back to without its picture.
func (s *Service) greetingMessage(update *models.Update) *bot.SendMessageParams {
	t := textsFor(update.Message)

	var name string
	if update.Message.From != nil {
		name = update.Message.From.FirstName
	}

	params := &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      t.greeting(name),
		ParseMode: models.ParseModeHTML,
	}

	if open, ok := s.siteButton(update.Message.Chat, t.openApp, "/"); ok {
		params.ReplyMarkup = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{open}}}
	}

	return params
}

func handleUnknownCmd(update *models.Update) outgoing {
	command, _ := parseCommand(update.Message)

	return buildMsg(update, fmt.Sprintf(textsFor(update.Message).unknownCommand, command))
}

// handleTimeCmd reads its clock as a pace: /time 4:50 10k.
func (s *Service) handleTimeCmd(update *models.Update) outgoing {
	t := textsFor(update.Message)

	return s.planCommand(update, plan.Pace, t.usageTime, t)
}

// handlePaceCmd reads its clock as a finish time: /pace 21.1 1:38:48.
func (s *Service) handlePaceCmd(update *models.Update) outgoing {
	t := textsFor(update.Message)

	return s.planCommand(update, plan.FinishTime, t.usagePace, t)
}

// handleCardCmd draws the plan as a picture, which forwards into a chat as one
// message and shows the splits a line of text cannot.
func (s *Service) handleCardCmd(update *models.Update) outgoing {
	t := textsFor(update.Message)

	_, arguments := parseCommand(update.Message)
	if strings.TrimSpace(arguments) == "" {
		return buildMsg(update, t.emptyArguments+"\n"+t.usageCard)
	}

	p, err := plan.ParseAs(arguments, plan.FinishTime)
	if err != nil {
		return buildMsg(update, t.problem(err, plan.FinishTime, plan.Unrecognized(arguments), t.usageCard))
	}

	photo, ok := s.cardPhoto(update.Message.Chat.ID, p, t)
	if !ok {
		return buildMsg(update, t.cardFailed)
	}

	return photoReply{params: photo}
}

func buildMsg(update *models.Update, text string) outgoing {
	return textReply{params: &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   text,
	}}
}
