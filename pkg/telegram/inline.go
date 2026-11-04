package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/card"
	"gorun/pkg/plan"
)

// ErrSharingUnavailable means HOST cannot provide an HTTPS plan link, or the
// Telegram client is not configured.
var ErrSharingUnavailable = errors.New("telegram sharing is not configured")

// inlinePlan is shared by inline queries and Mini App prepared messages. Inline
// messages deliberately use only a URL button: web_app buttons and chat-bound
// callbacks are not appropriate when the destination chat is unknown.
func (s *Service) inlinePlan(p plan.Plan, language string) *models.InlineQueryResultArticle {
	t := textsForLanguage(language)
	caption := card.Caption(card.Plan{Distance: p.Distance, Time: p.Time, Language: t.language})
	link := s.siteURL + planPath(p, t)
	return &models.InlineQueryResultArticle{
		ID:    fmt.Sprintf("%d:%d:%s", p.Distance, int(p.Time.Seconds()), t.language),
		Title: caption, URL: link,
		InputMessageContent: models.InputTextMessageContent{MessageText: caption},
		ReplyMarkup:         models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: t.openPlan, URL: link}}}},
	}
}

type inlineReply struct{ params *bot.AnswerInlineQueryParams }

func (r inlineReply) send(ctx context.Context, client *bot.Bot) error {
	_, err := client.AnswerInlineQuery(ctx, r.params)
	return err
}

func (s *Service) handleInline(update *models.Update) outgoing {
	query := update.InlineQuery
	params := &bot.AnswerInlineQueryParams{InlineQueryID: query.ID, Results: []models.InlineQueryResult{}, IsPersonal: true, CacheTime: 1}
	p, err := plan.Parse(query.Query)
	if err == nil && query.Offset == "" && s.siteURL != "" && validSharedPlan(p) {
		language := ""
		if query.From != nil {
			language = query.From.LanguageCode
		}
		params.Results = append(params.Results, s.inlinePlan(p, language))
	}
	return inlineReply{params: params}
}

func validSharedPlan(p plan.Plan) bool {
	return p.Distance > 0 && p.Distance <= maxDistance && p.Time >= time.Second && p.Time <= time.Duration(maxSeconds)*time.Second
}

// PrepareInlineMessage stores a message for the authenticated user. It sends
// nothing to a chat: the user chooses the destination in Telegram's dialog.
func (s *Service) PrepareInlineMessage(ctx context.Context, userID int64, p plan.Plan, language string) (*models.PreparedInlineMessage, error) {
	if s == nil || s.bot == nil || s.siteURL == "" {
		return nil, ErrSharingUnavailable
	}
	if userID <= 0 || !validSharedPlan(p) {
		return nil, errors.New("invalid shared plan or user")
	}
	return s.bot.SavePreparedInlineMessage(ctx, &bot.SavePreparedInlineMessageParams{
		UserID: userID, Result: s.inlinePlan(p, language), AllowUserChats: true, AllowGroupChats: true, AllowChannelChats: true,
	})
}
