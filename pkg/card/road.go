package card

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"

	"golang.org/x/image/vector"
)

// The rest of the page's palette, for pictures drawn on paper: ink and the
// softer ink of secondary words.
var (
	paperInk      = color.RGBA{R: 0x1E, G: 0x25, B: 0x22, A: 0xFF}
	paperInkMuted = color.RGBA{R: 0x5B, G: 0x66, B: 0x61, A: 0xFF}
	// The road edges are paper at 30 % over asphalt, as in icon.svg.
	roadEdge = color.NRGBA{R: 0xF3, G: 0xF4, B: 0xF1, A: 0x4D}
)

// Sizes of the pictures the bot and its profile show.
const (
	// AvatarSize is what BotFather takes for a profile photo.
	AvatarSize = 640
	// DescriptionWidth and DescriptionHeight are the picture an empty chat
	// shows above the description before /start.
	DescriptionWidth  = 640
	DescriptionHeight = 360
)

// The road of assets/icon.svg in its own 512 units: two edges running to the
// horizon and three yellow dashes of the centre line, shrinking with distance.
const iconUnits = 512

var (
	roadEdges = [2][2]point{
		{{48, 512}, {232, 64}},
		{{464, 512}, {280, 64}},
	}
	roadEdgeWidth = 12.0
	roadDashes    = [3][4]point{
		{{227.5, 440}, {284.5, 440}, {278.2, 345}, {233.8, 345}},
		{{236.6, 300}, {275.4, 300}, {271.4, 240}, {240.6, 240}},
		{{242.9, 205}, {269.1, 205}, {267.2, 175}, {244.8, 175}},
	}
)

type point struct{ x, y float64 }

// drawRoad paints the icon's road into a square of the picture, edge to edge:
// no rounded corners, so the square can be cut into a circle or set against
// paper.
func drawRoad(picture *image.RGBA, square image.Rectangle) {
	draw.Draw(picture, square, image.NewUniform(asphalt), image.Point{}, draw.Src)

	scale := float64(square.Dx()) / iconUnits
	at := func(p point) (float32, float32) {
		return float32(float64(square.Min.X) + p.x*scale), float32(float64(square.Min.Y) + p.y*scale)
	}

	fill := func(ink color.Color, corners ...point) {
		raster := vector.NewRasterizer(square.Dx(), square.Dy())
		raster.DrawOp = draw.Over
		for i, corner := range corners {
			x, y := at(corner)
			x, y = x-float32(square.Min.X), y-float32(square.Min.Y)
			if i == 0 {
				raster.MoveTo(x, y)
			} else {
				raster.LineTo(x, y)
			}
		}
		raster.ClosePath()
		raster.Draw(picture, square, image.NewUniform(ink), image.Point{})
	}

	// A stroke with flat ends is the quadrilateral around its centre line.
	for _, edge := range roadEdges {
		from, to := edge[0], edge[1]
		length := math.Hypot(to.x-from.x, to.y-from.y)
		nx, ny := (from.y-to.y)/length*roadEdgeWidth/2, (to.x-from.x)/length*roadEdgeWidth/2
		fill(roadEdge,
			point{from.x + nx, from.y + ny}, point{to.x + nx, to.y + ny},
			point{to.x - nx, to.y - ny}, point{from.x - nx, from.y - ny})
	}

	for _, dash := range roadDashes {
		fill(mark, dash[:]...)
	}
}

// Avatar is the icon for the bot's profile photo: the road edge to edge,
// since Telegram cuts the photo into a circle and the corners are lost anyway.
// The dashes sit well inside that circle.
func Avatar() ([]byte, error) {
	picture := image.NewRGBA(image.Rect(0, 0, AvatarSize, AvatarSize))
	drawRoad(picture, picture.Bounds())

	return encode(picture)
}

// Description is the picture over the description in an empty chat. It is
// the same for every language, so it carries the name and race distances
// rather than a sentence.
func Description() ([]byte, error) {
	fonts, err := loadTypefaces()
	if err != nil {
		return nil, err
	}

	picture := image.NewRGBA(image.Rect(0, 0, DescriptionWidth, DescriptionHeight))
	draw.Draw(picture, picture.Bounds(), image.NewUniform(paper), image.Point{}, draw.Src)
	drawRoad(picture, image.Rect(DescriptionWidth-DescriptionHeight, 0, DescriptionWidth, DescriptionHeight))

	pen := &pen{picture: picture, fonts: fonts}
	pen.write(36, 170, 64, fonts.bold, paperInk, "Pacer")
	draw.Draw(picture, image.Rect(38, 196, 86, 204), image.NewUniform(mark), image.Point{}, draw.Src)
	pen.write(36, 244, 22, fonts.regular, paperInkMuted, "5K · 10K · 21.1 · 42.2")

	if pen.err != nil {
		return nil, pen.err
	}

	return encode(picture)
}

// greetingWords are the lines the greeting picture carries in one language.
type greetingWords struct {
	headline []string
	subline  string
}

var greetings = map[string]greetingWords{
	"ru": {
		headline: []string{"Время и темп", "на любой", "дистанции"},
		subline:  "раскладка и прогноз результата",
	},
	"en": {
		headline: []string{"Time and pace", "for any", "distance"},
		subline:  "splits and a race prediction",
	},
}

// greetingTextWidth is the paper column left of the road, less the margin
// on the left and a narrower gap before the road.
const (
	greetingMargin    = 64
	greetingRoadGap   = 40
	greetingTextWidth = Width - Height - greetingMargin - greetingRoadGap
)

// Greeting is the picture the bot says hello with: words on paper beside the
// icon's road, in the page's colours. An unknown language is drawn in Russian.
func Greeting(language string) ([]byte, error) {
	fonts, err := loadTypefaces()
	if err != nil {
		return nil, err
	}

	words, ok := greetings[language]
	if !ok {
		words = greetings["ru"]
	}

	picture := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(picture, picture.Bounds(), image.NewUniform(paper), image.Point{}, draw.Src)
	drawRoad(picture, image.Rect(Width-Height, 0, Width, Height))

	pen := &pen{picture: picture, fonts: fonts}
	x := greetingMargin
	pen.write(x, 118, 40, fonts.bold, paperInk, "Pacer")

	for i, line := range words.headline {
		pen.write(x, 258+i*72, 64, fonts.bold, paperInk, line)
	}

	draw.Draw(picture, image.Rect(x+2, 444, x+82, 456), image.NewUniform(mark), image.Point{}, draw.Src)
	pen.write(x, 524, 30, fonts.regular, paperInkMuted, words.subline)

	if pen.err != nil {
		return nil, pen.err
	}

	return encode(picture)
}

func encode(picture image.Image) ([]byte, error) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		return nil, fmt.Errorf("encode the picture: %w", err)
	}

	return encoded.Bytes(), nil
}
