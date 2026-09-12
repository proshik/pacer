// Package card draws a plan as a picture for a chat: the distance, the time
// and the pace in the page's own colours.
package card

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"gorun/pkg/calculator"
)

// 1200x630 is what chats and link previews expect from a shared picture.
const (
	Width  = 1200
	Height = 630
)

// The page's palette: asphalt, paper, and the lane-marking yellow that marks
// a position and never carries text.
var (
	asphalt = color.RGBA{R: 0x1E, G: 0x25, B: 0x22, A: 0xFF}
	paper   = color.RGBA{R: 0xF3, G: 0xF4, B: 0xF1, A: 0xFF}
	inkSoft = color.RGBA{R: 0x9D, G: 0xA8, B: 0xA2, A: 0xFF}
	mark    = color.RGBA{R: 0xFF, G: 0xD2, B: 0x1F, A: 0xFF}
)

// Plan is what the picture shows. An unknown language is drawn in Russian.
type Plan struct {
	Distance int // meters
	Time     time.Duration
	Language string
}

// The card needs only a handful of words; the page and the link preview keep
// their own copies of these, since none of the three can import the others.
type texts struct {
	half        string
	marathon    string
	km          string
	decimal     string
	perKm       string
	paceLabel   string
	splitsLabel string
	finish      string
	caption     func(distance, total, pace string) string
}

var languages = map[string]texts{
	"ru": {
		half: "Полумарафон", marathon: "Марафон", km: "км", decimal: ",", perKm: "/км",
		paceLabel: "темп", splitsLabel: "раскладка", finish: "финиш",
		caption: func(distance, total, pace string) string {
			return distance + " за " + total + " — это " + pace + " на километр"
		},
	},
	"en": {
		half: "Half marathon", marathon: "Marathon", km: "km", decimal: ".", perKm: "/km",
		paceLabel: "pace", splitsLabel: "splits", finish: "finish",
		caption: func(distance, total, pace string) string {
			return distance + " in " + total + " — that's " + pace + " per kilometer"
		},
	},
}

// Caption says the same plan in words, for a reader who cannot see the card.
func Caption(plan Plan) string {
	if plan.Distance <= 0 || plan.Time <= 0 {
		return ""
	}

	words := wordsFor(plan.Language)
	pace := calculator.NewService().Pace(plan.Distance, plan.Time)

	return words.caption(words.distanceName(plan.Distance), formatClock(plan.Time), formatClock(pace))
}

func wordsFor(language string) texts {
	if words, ok := languages[language]; ok {
		return words
	}

	return languages["ru"]
}

// The Go fonts carry Cyrillic and ship as a module, so no font file lives in
// this repository. They are parsed once.
type typefaces struct {
	regular *opentype.Font
	bold    *opentype.Font
}

var loadTypefaces = sync.OnceValues(func() (typefaces, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return typefaces{}, fmt.Errorf("parse the regular font: %w", err)
	}

	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return typefaces{}, fmt.Errorf("parse the bold font: %w", err)
	}

	return typefaces{regular: regular, bold: bold}, nil
})

// Render draws the plan and encodes it as PNG.
func Render(plan Plan) ([]byte, error) {
	if plan.Distance <= 0 {
		return nil, errors.New("distance must be greater than zero")
	}
	if plan.Time <= 0 {
		return nil, errors.New("time must be greater than zero")
	}

	fonts, err := loadTypefaces()
	if err != nil {
		return nil, err
	}

	words := wordsFor(plan.Language)

	picture := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(picture, picture.Bounds(), image.NewUniform(asphalt), image.Point{}, draw.Src)

	// The yellow bar marks the plan the way it marks every 5 km in the splits.
	draw.Draw(picture, image.Rect(60, 170, 72, 420), image.NewUniform(mark), image.Point{}, draw.Src)

	pace := calculator.NewService().Pace(plan.Distance, plan.Time)
	pen := &pen{picture: picture, fonts: fonts}

	pen.write(110, 110, 34, fonts.regular, inkSoft, "Pacer")
	pen.write(110, 230, 64, fonts.bold, paper, words.distanceName(plan.Distance))
	pen.write(110, 390, 150, fonts.bold, paper, formatClock(plan.Time))
	pen.write(110, 480, 32, fonts.regular, inkSoft, words.paceLabel)
	paceWidth := pen.write(110, 560, 76, fonts.bold, paper, formatClock(pace))
	pen.write(110+paceWidth+16, 560, 40, fonts.regular, inkSoft, words.perKm)

	pen.splits(plan, words)

	if pen.err != nil {
		return nil, pen.err
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		return nil, fmt.Errorf("encode the card: %w", err)
	}

	return encoded.Bytes(), nil
}

// pen writes lines of text and keeps the first failure, so the drawing reads
// as a sequence of lines rather than a chain of error checks.
type pen struct {
	picture *image.RGBA
	fonts   typefaces
	err     error
}

// write draws text with its baseline at y and returns how wide it came out.
func (p *pen) write(x, y int, size float64, typeface *opentype.Font, ink color.Color, text string) int {
	if p.err != nil {
		return 0
	}

	face, err := opentype.NewFace(typeface, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		p.err = fmt.Errorf("make a %.0fpt face: %w", size, err)
		return 0
	}
	defer func() { _ = face.Close() }()

	drawer := &font.Drawer{
		Dst:  p.picture,
		Src:  image.NewUniform(ink),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)

	return drawer.Dot.X.Round() - x
}

// measure is how wide text would come out, for laying it out right aligned.
func (p *pen) measure(size float64, typeface *opentype.Font, text string) int {
	if p.err != nil {
		return 0
	}

	face, err := opentype.NewFace(typeface, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		p.err = fmt.Errorf("make a %.0fpt face: %w", size, err)
		return 0
	}
	defer func() { _ = face.Close() }()

	return font.MeasureString(face, text).Round()
}

// splits fills the right half with the time at every mark, which is what the
// picture carries beyond the sentence. A race with nothing to split — one mark
// or fewer — leaves that half empty.
func (p *pen) splits(plan Plan, words texts) {
	step, marks := splitStep(plan.Distance)
	if marks < 2 {
		return
	}

	const (
		labelX  = 700
		timeX   = 1140
		firstY  = 208
		rowStep = 48
	)

	p.write(labelX, 150, 28, p.fonts.regular, inkSoft, words.splitsLabel)

	for i := 1; i <= marks; i++ {
		mark := i * step
		label := strconv.Itoa(mark/1000) + " " + words.km
		ink := inkSoft
		if mark >= plan.Distance {
			mark, label, ink = plan.Distance, words.finish, paper
		}

		// Even pacing: the time at a mark is the finish time in proportion.
		elapsed := time.Duration(int64(plan.Time) * int64(mark) / int64(plan.Distance))
		y := firstY + (i-1)*rowStep

		p.write(labelX, y, 30, p.fonts.regular, ink, label)
		value := formatClock(elapsed)
		p.write(timeX-p.measure(32, p.fonts.bold, value), y, 32, p.fonts.bold, paper, value)
	}
}

// splitStep picks the coarsest useful mark spacing that keeps the column short
// enough to fit, and counts the rows including the finish.
func splitStep(distance int) (step, marks int) {
	const maxRows = 9

	for _, step := range []int{1000, 2000, 5000, 10000, 20000} {
		rows := distance / step
		if distance%step != 0 {
			rows++ // the finish gets its own row
		}
		if rows <= maxRows {
			return step, rows
		}
	}

	return 0, 0
}

func (t texts) distanceName(meters int) string {
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
