package card

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

func decode(t *testing.T, data []byte, err error) image.Image {
	t.Helper()

	if err != nil {
		t.Fatalf("draw: %v", err)
	}

	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	return picture
}

func sameColor(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()

	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func assertSize(t *testing.T, picture image.Image, width, height int) {
	t.Helper()

	if got := picture.Bounds().Size(); got != image.Pt(width, height) {
		t.Errorf("size = %v, want %dx%d", got, width, height)
	}
}

// Telegram cuts the profile photo into a circle. The asphalt runs to every
// edge, so no light corner shows through, and the yellow marking stays well
// inside the circle, in the central 80 % a maskable icon keeps.
func TestAvatarKeepsTheMarkingInsideTheCircle(t *testing.T) {
	data, err := Avatar()
	picture := decode(t, data, err)
	assertSize(t, picture, AvatarSize, AvatarSize)

	last := AvatarSize - 1
	for _, corner := range []image.Point{{0, 0}, {last, 0}, {0, last}, {last, last}} {
		if got := picture.At(corner.X, corner.Y); !sameColor(got, asphalt) {
			t.Errorf("corner %v = %v, want asphalt", corner, got)
		}
	}

	centre, safe := float64(AvatarSize)/2, 0.8*float64(AvatarSize)/2
	var yellow int
	for y := range AvatarSize {
		for x := range AvatarSize {
			if !sameColor(picture.At(x, y), mark) {
				continue
			}
			yellow++
			if math.Hypot(float64(x)+0.5-centre, float64(y)+0.5-centre) > safe {
				t.Fatalf("marking at (%d, %d) is outside the safe circle", x, y)
			}
		}
	}
	if yellow < 1000 {
		t.Errorf("only %d yellow pixels; the centre line is missing", yellow)
	}
}

// The description picture is paper with the road on the right, the page and
// the icon side by side.
func TestDescriptionPictureIsPaperBesideTheRoad(t *testing.T) {
	data, err := Description()
	picture := decode(t, data, err)
	assertSize(t, picture, DescriptionWidth, DescriptionHeight)

	if got := picture.At(0, 0); !sameColor(got, paper) {
		t.Errorf("left corner = %v, want paper", got)
	}
	if got := picture.At(DescriptionWidth-1, 0); !sameColor(got, asphalt) {
		t.Errorf("right corner = %v, want asphalt", got)
	}
	if !hasColor(picture, paperInk) || !hasColor(picture, mark) {
		t.Error("the picture lacks the ink of its name or the yellow marking")
	}
}

func TestGreetingPictureIsDrawnPerLanguage(t *testing.T) {
	pictures := map[string][]byte{}
	for _, language := range []string{"ru", "en"} {
		data, err := Greeting(language)
		picture := decode(t, data, err)
		assertSize(t, picture, Width, Height)

		if got := picture.At(0, 0); !sameColor(got, paper) {
			t.Errorf("%s: left corner = %v, want paper", language, got)
		}
		if got := picture.At(Width-1, Height-1); !sameColor(got, asphalt) {
			t.Errorf("%s: right corner = %v, want asphalt", language, got)
		}
		if coverage := inkIn(picture, image.Rect(0, 0, Width-Height, Height)); coverage < 10000 {
			t.Errorf("%s: only %d pixels of words on the paper", language, coverage)
		}
		pictures[language] = data
	}

	if bytes.Equal(pictures["ru"], pictures["en"]) {
		t.Error("the Russian and English greetings came out the same")
	}
}

// The words of the greeting picture must fit the paper column in every
// language; a longer translation would run onto the road.
func TestGreetingWordsFitThePaper(t *testing.T) {
	fonts, err := loadTypefaces()
	if err != nil {
		t.Fatal(err)
	}

	p := &pen{fonts: fonts}
	for language, words := range greetings {
		for _, line := range words.headline {
			if width := p.measure(64, fonts.bold, line); width > greetingTextWidth {
				t.Errorf("%s headline %q is %d px wide, the paper holds %d", language, line, width, greetingTextWidth)
			}
		}
		if width := p.measure(30, fonts.regular, words.subline); width > greetingTextWidth {
			t.Errorf("%s subline %q is %d px wide, the paper holds %d", language, words.subline, width, greetingTextWidth)
		}
	}
	if p.err != nil {
		t.Fatal(p.err)
	}
}
