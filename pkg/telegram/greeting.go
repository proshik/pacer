package telegram

import (
	"bytes"
	"context"
	"log/slog"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/card"
)

// greetingPicture draws the greeting picture of a language once per process.
var greetingPicture = map[string]func() ([]byte, error){
	"ru": sync.OnceValues(func() ([]byte, error) { return card.Greeting("ru") }),
	"en": sync.OnceValues(func() ([]byte, error) { return card.Greeting("en") }),
}

// photoCache keeps the file_id Telegram returns for an uploaded picture; the
// next message sends that id instead of the bytes.
type photoCache struct {
	mu  sync.Mutex
	ids map[string]string
}

func newPhotoCache() *photoCache {
	return &photoCache{ids: map[string]string{}}
}

func (c *photoCache) get(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ids[key]
}

func (c *photoCache) set(key, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ids[key] = id
}

func (c *photoCache) forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.ids, key)
}

// greetingReply sends the greeting as a photo with the words as its caption.
// The bot must not stay silent on /start, so every way the photo can fail
// ends in the same words as a plain message: a stale file_id is uploaded
// afresh, and a failed upload or drawing falls back to text.
type greetingReply struct {
	photo    *bot.SendPhotoParams // everything but the photo itself
	text     *bot.SendMessageParams
	language string
	cache    *photoCache
}

func (r greetingReply) send(ctx context.Context, client *bot.Bot) error {
	if id := r.cache.get(r.language); id != "" {
		err := r.sendPhoto(ctx, client, &models.InputFileString{Data: id})
		if err == nil || ctx.Err() != nil {
			return err
		}

		slog.Warn("send greeting picture by file_id failed", "err", err, "language", r.language)
		r.cache.forget(r.language)
	}

	if picture, err := r.picture(); err != nil {
		slog.Warn("draw greeting picture failed", "err", err, "language", r.language)
	} else {
		err := r.sendPhoto(ctx, client, &models.InputFileUpload{Filename: "pacer.png", Data: bytes.NewReader(picture)})
		if err == nil || ctx.Err() != nil {
			return err
		}

		slog.Warn("upload greeting picture failed", "err", err, "language", r.language)
	}

	_, err := client.SendMessage(ctx, r.text)

	return err
}

func (r greetingReply) picture() ([]byte, error) {
	draw, ok := greetingPicture[r.language]
	if !ok {
		draw = greetingPicture["en"]
	}

	return draw()
}

// sendPhoto sends the greeting with the given photo and, on success, keeps the
// file_id of the largest size Telegram stored.
func (r greetingReply) sendPhoto(ctx context.Context, client *bot.Bot, photo models.InputFile) error {
	params := *r.photo
	params.Photo = photo

	message, err := client.SendPhoto(ctx, &params)
	if err != nil {
		return err
	}

	if message != nil && len(message.Photo) > 0 {
		r.cache.set(r.language, message.Photo[len(message.Photo)-1].FileID)
	}

	return nil
}
