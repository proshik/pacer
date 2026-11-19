package telegram

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

// DefaultWebhookCheckInterval is how often WatchWebhook asks Telegram about
// the webhook unless told otherwise.
const DefaultWebhookCheckInterval = 5 * time.Minute

// webhookCheckTimeout bounds one getWebhookInfo call.
const webhookCheckTimeout = 10 * time.Second

// WatchWebhook asks Telegram about the webhook right away and then every
// interval, and records the answer in the webhook metrics: whether the webhook
// still points at this deployment, how many updates wait undelivered and when
// a delivery last failed. A pod can be healthy and still deaf if someone or
// something removed or moved the webhook; only Telegram knows.
//
// It only reads the webhook and never sets or removes it. It does nothing in
// polling mode, without metrics or with a non-positive interval. Close stops
// it. Call it once, before Close.
func (s *Service) WatchWebhook(interval time.Duration) {
	if s.polling || s.webhookURL == "" || s.metrics == nil || interval <= 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelWatch = cancel

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var lastErrorSeen int
		for {
			lastErrorSeen = s.checkWebhook(ctx, lastErrorSeen)

			select {
			case <-ctx.Done():
				return
			case <-s.done:
				return
			case <-ticker.C:
			}
		}
	}()

	slog.Info("telegram webhook check started", "interval", interval)
}

// checkWebhook makes one getWebhookInfo call and records it. lastErrorSeen is
// the last delivery error date already logged, so one failure is logged once;
// the date seen now is returned.
//
// Telegram's webhook URL contains the bot token, so neither the URL nor
// anything built from it is logged.
func (s *Service) checkWebhook(ctx context.Context, lastErrorSeen int) int {
	callCtx, cancel := context.WithTimeout(ctx, webhookCheckTimeout)
	defer cancel()

	info, err := s.bot.GetWebhookInfo(callCtx)
	if err != nil {
		if ctx.Err() != nil {
			// Shutting down: not Telegram's failure.
			return lastErrorSeen
		}

		// The library masks the token in transport errors.
		s.metrics.WebhookCheckFailed()
		slog.Warn("telegram webhook check failed", "err", err)

		return lastErrorSeen
	}

	ours := sameWebhookURL(info.URL, s.webhookURL)

	var lastError time.Time
	if info.LastErrorDate > 0 {
		lastError = time.Unix(int64(info.LastErrorDate), 0)
	}

	s.metrics.WebhookChecked(ours, info.PendingUpdateCount, lastError)

	if !ours {
		slog.Warn("telegram webhook points elsewhere", "webhook_set", info.URL != "", "pending_updates", info.PendingUpdateCount)
	}

	if info.LastErrorDate > 0 && info.LastErrorDate != lastErrorSeen {
		slog.Warn("telegram webhook delivery failed",
			"at", lastError.UTC().Format(time.RFC3339),
			"message", info.LastErrorMessage,
			"pending_updates", info.PendingUpdateCount,
		)
	}

	return info.LastErrorDate
}

// sameWebhookURL compares the webhook Telegram reports with ours the way a
// URL compares: scheme and host case-insensitively and the default https port
// optional, the path exactly. An empty or unparsable URL is never ours.
func sameWebhookURL(reported, ours string) bool {
	a, errA := url.Parse(reported)
	b, errB := url.Parse(ours)
	if errA != nil || errB != nil || reported == "" || ours == "" {
		return false
	}

	return strings.EqualFold(a.Scheme, b.Scheme) &&
		canonicalHost(a) == canonicalHost(b) &&
		a.EscapedPath() == b.EscapedPath() &&
		a.RawQuery == b.RawQuery
}

func canonicalHost(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" && port != "443" {
		return host + ":" + port
	}

	return host
}
