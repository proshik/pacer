package rest

import (
	"bytes"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorun/pkg/calculator"
)

// The page marks its link preview tags with these comments. The server swaps
// the block for one that carries the plan from the shared link, so a messenger
// shows "Марафон за 3:44:20 — это 5:19 на километр" instead of the site name.
const (
	previewStart = "<!-- og:start -->"
	previewEnd   = "<!-- og:end -->"
)

// maxPlanHours matches the hours field on the page.
const maxPlanHours = 999

type previewTexts struct {
	locale      string
	description string // no plan in the link
	planNote    string // description under a plan
	half        string
	marathon    string
	km          string
	decimal     string
	plan        func(distance, total, pace string) string
}

// Phrased as the page's own header phrases the plan.
var previewLanguages = map[string]previewTexts{
	"ru": {
		locale:      "ru_RU",
		description: "Беговой калькулятор: время и темп на любой дистанции, раскладка по километрам, прогноз результата и VDOT.",
		planNote:    "Раскладка по километрам, прогноз на другие дистанции и тренировочные темпы.",
		half:        "Полумарафон",
		marathon:    "Марафон",
		km:          "км",
		decimal:     ",",
		plan: func(distance, total, pace string) string {
			return distance + " за " + total + " — это " + pace + " на километр"
		},
	},
	"en": {
		locale:      "en_US",
		description: "Running calculator: time and pace for any distance, kilometer splits, a race prediction and VDOT.",
		planNote:    "Splits by kilometer, a prediction for other distances and training paces.",
		half:        "Half marathon",
		marathon:    "Marathon",
		km:          "km",
		decimal:     ".",
		plan: func(distance, total, pace string) string {
			return distance + " in " + total + " — that's " + pace + " per kilometer"
		},
	},
}

// pageHandler serves index.html with the link preview filled in from the query
// string. A page without the markers is served unchanged.
func pageHandler(page []byte, site string, c calculator.Engine) http.HandlerFunc {
	head, tail, found := splitPreview(page)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if !found {
			_, _ = w.Write(page)
			return
		}

		w.Header().Add("Vary", "Accept-Language")

		var body bytes.Buffer
		body.Write(head)
		body.WriteString(previewBlock(r, site, c))
		body.Write(tail)
		_, _ = w.Write(body.Bytes())
	}
}

func splitPreview(page []byte) (head, tail []byte, found bool) {
	start := bytes.Index(page, []byte(previewStart))
	end := bytes.Index(page, []byte(previewEnd))
	if start == -1 || end < start {
		return nil, nil, false
	}

	return page[:start], page[end+len(previewEnd):], true
}

// previewBlock builds the tags from parsed numbers only, never from the raw
// query, so nothing a link carries can reach the markup.
func previewBlock(r *http.Request, site string, c calculator.Engine) string {
	query := r.URL.Query()
	texts := previewLanguages[previewLanguage(query.Get("l"), r.Header.Get("Accept-Language"))]

	title, description, path := "Pacer", texts.description, "/"
	if dist, total, ok := parsePlan(query.Get("d"), query.Get("t")); ok {
		title = texts.plan(texts.distanceName(dist), formatClock(total), formatClock(c.Pace(dist, total)))
		description = texts.planNote
		path = fmt.Sprintf("/?d=%d&t=%s", dist, formatHMS(total))
	}

	image := "icon-512.png"
	if site != "" {
		image = site + "/icon-512.png"
	}

	tags := [][2]string{
		{"og:type", "website"},
		{"og:site_name", "Pacer"},
		{"og:title", title},
		{"og:description", description},
	}
	if site != "" {
		tags = append(tags, [2]string{"og:url", site + path})
	}
	tags = append(tags,
		[2]string{"og:image", image},
		[2]string{"og:image:width", "512"},
		[2]string{"og:image:height", "512"},
		[2]string{"og:locale", texts.locale},
	)

	var block strings.Builder
	block.WriteString(previewStart + "\n")
	for _, tag := range tags {
		fmt.Fprintf(&block, "    <meta property=\"%s\" content=\"%s\">\n", tag[0], html.EscapeString(tag[1]))
	}
	block.WriteString("    <meta name=\"twitter:card\" content=\"summary\">\n")
	block.WriteString("    " + previewEnd)

	return block.String()
}

// previewLanguage prefers l, the sharer's language that the page puts into a
// copied link, because crawlers rarely send Accept-Language. The header is
// read in the order given; Russian is the default, as every link shared before
// l existed was Russian.
func previewLanguage(l string, acceptLanguage string) string {
	if _, ok := previewLanguages[l]; ok {
		return l
	}

	for _, part := range strings.Split(acceptLanguage, ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if _, ok := previewLanguages[base]; ok {
			return base
		}
	}

	return "ru"
}

// parsePlan reads d (meters) and t the way the page does: "1:38:48", "38:48"
// or a bare number of seconds.
func parsePlan(d, t string) (int, time.Duration, bool) {
	dist, err := strconv.Atoi(d)
	if err != nil || dist <= 0 {
		return 0, 0, false
	}

	seconds, ok := parseClock(t)
	if !ok || seconds <= 0 {
		return 0, 0, false
	}

	return dist, time.Duration(seconds) * time.Second, true
}

func parseClock(value string) (int, bool) {
	parts := strings.Split(value, ":")
	if len(parts) > 3 {
		return 0, false
	}

	numbers := make([]int, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return 0, false
		}

		number, err := strconv.Atoi(part)
		if err != nil {
			return 0, false
		}
		numbers[i] = number
	}

	if len(numbers) == 1 {
		if numbers[0] > maxPlanHours*3600 {
			return 0, false
		}
		return numbers[0], true
	}

	for len(numbers) < 3 {
		numbers = append([]int{0}, numbers...)
	}
	hours, minutes, seconds := numbers[0], numbers[1], numbers[2]
	if hours > maxPlanHours || minutes > 59 || seconds > 59 {
		return 0, false
	}

	return hours*3600 + minutes*60 + seconds, true
}

func (t previewTexts) distanceName(meters int) string {
	switch meters {
	case 21097:
		return t.half
	case 42195:
		return t.marathon
	}

	number := strconv.Itoa(meters / 1000)
	if rest := meters % 1000; rest != 0 {
		number += t.decimal + strings.TrimRight(fmt.Sprintf("%03d", rest), "0")
	}

	return number + " " + t.km
}

// formatClock is the running notation the page uses: 1:25:39 or 4:04.
func formatClock(d time.Duration) string {
	hours, minutes, seconds := calculator.Split(d)
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}

	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// formatHMS is the t= form the page writes into its own address.
func formatHMS(d time.Duration) string {
	hours, minutes, seconds := calculator.Split(d)

	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
}

// siteURL turns HOST into the absolute base for og:url and og:image. HOST comes
// with or without a scheme, as for the webhook. A value that cannot be used
// leaves the preview with relative links rather than failing startup.
func siteURL(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}

	parsed, err := url.Parse(host)
	if err != nil || parsed.Host == "" {
		slog.Warn("HOST is not a usable site address, link previews use relative URLs", "host", host)
		return ""
	}

	return parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/")
}
