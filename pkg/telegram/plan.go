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
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/analysis"
	"gorun/pkg/card"
	"gorun/pkg/plan"
)

// The views a plan's buttons switch between. The view travels in a button's
// callback data together with the plan: "splits:21097:5928".
const (
	viewPlan    = "plan"
	viewSplits  = "splits"
	viewPredict = "predict"
	viewCard    = "card"
)

// Callback data comes from the client and can be forged, so a plan read from
// it is held to sane limits before anything is drawn.
const (
	maxDistance = 1_000_000 // 1000 km
	maxSeconds  = 1000 * 60 * 60
	splitRows   = 25
)

// Telegram's HTML mode needs only these three escaped; the plan sentence has
// apostrophes that must stay as they are.
var escapeHTML = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace

// planCommand answers /pace and /time: the arguments are a plan whose clock
// means what the command says.
func (s *Service) planCommand(update *models.Update, kind plan.Kind, usage string, t *texts) outgoing {
	_, arguments := parseCommand(update.Message)
	if strings.TrimSpace(arguments) == "" {
		return buildMsg(update, t.emptyArguments+"\n"+usage)
	}

	p, err := plan.ParseAs(arguments, kind)
	if err != nil {
		return buildMsg(update, t.problem(err, kind, plan.Unrecognized(arguments), usage))
	}

	return s.planReply(update.Message.Chat, p, t)
}

// handleText reads a plain message as a plan; anything else gets the greeting.
func (s *Service) handleText(update *models.Update) outgoing {
	p, err := plan.Parse(update.Message.Text)
	if err != nil {
		return s.handleStartCmd(update)
	}

	return s.planReply(update.Message.Chat, p, textsFor(update.Message))
}

func (s *Service) planReply(chat models.Chat, p plan.Plan, t *texts) outgoing {
	return textReply{params: &bot.SendMessageParams{
		ChatID:      chat.ID,
		Text:        viewText(viewPlan, p, t),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: s.planKeyboard(chat, p, t, viewPlan),
	}}
}

// siteButton opens the page: as a Mini App in a private chat, the only place
// Telegram accepts web_app buttons, and as a plain link anywhere else. There
// is no button when the site cannot be opened from Telegram.
func (s *Service) siteButton(chat models.Chat, text, path string) (models.InlineKeyboardButton, bool) {
	if s.siteURL == "" {
		return models.InlineKeyboardButton{}, false
	}

	button := models.InlineKeyboardButton{Text: text}
	if chat.Type == models.ChatTypePrivate {
		button.WebApp = &models.WebAppInfo{URL: s.siteURL + path}
	} else {
		button.URL = s.siteURL + path
	}

	return button, true
}

// planKeyboard offers the views other than the one on screen, the card, and
// the app opened on this plan.
func (s *Service) planKeyboard(chat models.Chat, p plan.Plan, t *texts, current string) models.InlineKeyboardMarkup {
	data := func(view string) string {
		return fmt.Sprintf("%s:%d:%d", view, p.Distance, int(p.Time.Seconds()))
	}
	button := func(text, view string) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: text, CallbackData: data(view)}
	}

	var row []models.InlineKeyboardButton
	if current != viewPlan {
		row = append(row, button(t.backButton, viewPlan))
	}
	if current != viewSplits {
		row = append(row, button(t.splitsButton, viewSplits))
	}
	if current != viewPredict {
		row = append(row, button(t.predictButton, viewPredict))
	}
	row = append(row, button(t.cardButton, viewCard))

	rows := [][]models.InlineKeyboardButton{row}
	if open, ok := s.siteButton(chat, t.openPlan, planPath(p, t)); ok {
		rows = append(rows, []models.InlineKeyboardButton{open})
	}

	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// planPath opens the page on this plan with the parameters the page and its
// link preview read.
func planPath(p plan.Plan, t *texts) string {
	return fmt.Sprintf("/?d=%d&t=%s&l=%s", p.Distance, hmsText(p.Time), t.language)
}

// viewText is the plan sentence in bold, with the splits or the prediction
// under it as a monospace table.
func viewText(view string, p plan.Plan, t *texts) string {
	caption := card.Caption(card.Plan{Distance: p.Distance, Time: p.Time, Language: t.language})
	head := "<b>" + escapeHTML(caption) + "</b>"

	switch view {
	case viewSplits:
		return head + "\n\n" + t.splitsLabel + "\n<pre>" + escapeHTML(t.splitsTable(p)) + "</pre>"
	case viewPredict:
		return head + "\n\n" + t.predictLabel + "\n<pre>" + escapeHTML(t.predictionTable(p)) + "</pre>"
	default:
		return head
	}
}

func (t *texts) splitsTable(p plan.Plan) string {
	splits := p.Splits(plan.SplitStep(p.Distance, splitRows))

	lines := make([]string, 0, len(splits))
	for i, split := range splits {
		label := t.kilometres(split.Distance)
		if i == len(splits)-1 {
			label = t.finish
		}
		lines = append(lines, fmt.Sprintf("%-8s %8s", label, clockText(split.Elapsed)))
	}

	return strings.Join(lines, "\n")
}

func (t *texts) predictionTable(p plan.Plan) string {
	lines := []string{fmt.Sprintf("%-13s %8s %8s", t.distanceHeader, t.riegel, t.cameron)}
	for _, prediction := range analysis.Predict(p.Distance, p.Time).Predictions {
		lines = append(lines, fmt.Sprintf("%-13s %8s %8s",
			t.distanceName(prediction.Distance),
			clockText(time.Duration(prediction.Riegel.Seconds)*time.Second),
			clockText(time.Duration(prediction.Cameron.Seconds)*time.Second)))
	}

	return strings.Join(lines, "\n")
}

// cardPhoto draws the plan as the picture /card and the card button send.
func (s *Service) cardPhoto(chatID int64, p plan.Plan, t *texts) (*bot.SendPhotoParams, bool) {
	cardPlan := card.Plan{Distance: p.Distance, Time: p.Time, Language: t.language}

	picture, err := card.Render(cardPlan)
	if err != nil {
		slog.Warn("draw card failed", "err", err, "dist", p.Distance, "time", p.Time)
		return nil, false
	}

	return &bot.SendPhotoParams{
		ChatID:  chatID,
		Photo:   &models.InputFileUpload{Filename: "pacer.png", Data: bytes.NewReader(picture)},
		Caption: card.Caption(cardPlan),
	}, true
}

// callbackReply answers a tap on a button: the answer stops the button's
// spinner, and then the message is rewritten or the card is sent.
type callbackReply struct {
	answer *bot.AnswerCallbackQueryParams
	edit   *bot.EditMessageTextParams
	photo  *bot.SendPhotoParams
}

func (r callbackReply) send(ctx context.Context, client *bot.Bot) error {
	_, answerErr := client.AnswerCallbackQuery(ctx, r.answer)

	var err error
	switch {
	case r.edit != nil:
		_, err = client.EditMessageText(ctx, r.edit)
	case r.photo != nil:
		_, err = client.SendPhoto(ctx, r.photo)
	}

	return errors.Join(answerErr, err)
}

// handleCallback switches the view of the plan in the message the button is
// under, or sends the plan's card.
func (s *Service) handleCallback(update *models.Update) outgoing {
	query := update.CallbackQuery
	reply := callbackReply{answer: &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}}

	view, p, ok := parseCallback(query.Data)
	message := query.Message.Message
	if !ok || message == nil {
		return reply
	}

	t := textsForLanguage(query.From.LanguageCode)

	if view == viewCard {
		photo, drawn := s.cardPhoto(message.Chat.ID, p, t)
		if !drawn {
			reply.answer.Text = t.cardFailed
			return reply
		}

		reply.photo = photo
		return reply
	}

	reply.edit = &bot.EditMessageTextParams{
		ChatID:      message.Chat.ID,
		MessageID:   message.ID,
		Text:        viewText(view, p, t),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: s.planKeyboard(message.Chat, p, t, view),
	}

	return reply
}

func parseCallback(data string) (string, plan.Plan, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		return "", plan.Plan{}, false
	}

	switch parts[0] {
	case viewPlan, viewSplits, viewPredict, viewCard:
	default:
		return "", plan.Plan{}, false
	}

	meters, err := strconv.Atoi(parts[1])
	if err != nil || meters <= 0 || meters > maxDistance {
		return "", plan.Plan{}, false
	}

	seconds, err := strconv.Atoi(parts[2])
	if err != nil || seconds <= 0 || seconds > maxSeconds {
		return "", plan.Plan{}, false
	}

	return parts[0], plan.Plan{Distance: meters, Time: time.Duration(seconds) * time.Second}, true
}

// webAppSite is the https address the page is served from, or "" when HOST is
// not https: Telegram opens web_app buttons over https only.
func webAppSite(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}

	parsed, err := url.Parse(host)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}

	return "https://" + parsed.Host + strings.TrimRight(parsed.Path, "/")
}

// clockText is the running notation: 1:38:48 or 4:41.
func clockText(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	hours, minutes, seconds := total/3600, total%3600/60, total%60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}

	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// hmsText is the t= form the page reads: always hours, minutes and seconds.
func hmsText(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())

	return fmt.Sprintf("%d:%02d:%02d", total/3600, total%3600/60, total%60)
}
