package rest

import (
	"log/slog"
	"net/http"

	"gorun/pkg/card"
)

// cardImage GET /api/v1/card.png?d=42195&t=3:44:20&l=ru
//
// The plan as a picture: the same link parameters as the page, so a chat or a
// link preview can show the plan without opening it.
func cardImage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	dist, total, ok := parsePlan(query.Get("d"), query.Get("t"))
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "validation failed", map[string]string{
			"d": "distance in meters, greater than zero",
			"t": "time as 3:44:20, 44:20 or a number of seconds",
		})
		return
	}

	picture, err := card.Render(card.Plan{
		Distance: dist,
		Time:     total,
		Language: previewLanguage(query.Get("l"), r.Header.Get("Accept-Language")),
	})
	if err != nil {
		slog.Error("draw card failed", "err", err, "dist", dist, "time", total)
		writeJSONError(w, http.StatusInternalServerError, "could not draw the card", nil)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	// The picture depends only on the link, so it can be cached for a day.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Add("Vary", "Accept-Language")

	if _, err := w.Write(picture); err != nil {
		slog.Warn("write card failed", "err", err)
	}
}
