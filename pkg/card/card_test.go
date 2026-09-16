package card

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

func render(t *testing.T, plan Plan) ([]byte, image.Image) {
	t.Helper()

	data, err := Render(plan)
	if err != nil {
		t.Fatalf("render %+v: %v", plan, err)
	}

	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	return data, picture
}

func marathon() Plan {
	return Plan{Distance: 42195, Time: 3*time.Hour + 44*time.Minute + 20*time.Second, Language: "ru"}
}

// ink counts the pixels that differ from the background, which is how these
// tests see that something was actually drawn.
func ink(picture image.Image) int {
	return inkIn(picture, picture.Bounds())
}

func inkIn(picture image.Image, area image.Rectangle) int {
	background := picture.At(picture.Bounds().Min.X, picture.Bounds().Min.Y)

	count := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if picture.At(x, y) != background {
				count++
			}
		}
	}

	return count
}

func hasColor(picture image.Image, want color.Color) bool {
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if picture.At(x, y) == want {
				return true
			}
		}
	}

	return false
}

// The card goes into a chat as a photo and into link previews, where 1200x630
// is what every client expects.
func TestCardIsAPNGWithTheExpectedSize(t *testing.T) {
	_, picture := render(t, marathon())

	if got, want := picture.Bounds().Dx(), Width; got != want {
		t.Errorf("width = %d, want %d", got, want)
	}
	if got, want := picture.Bounds().Dy(), Height; got != want {
		t.Errorf("height = %d, want %d", got, want)
	}
}

// The card is the page's world: asphalt, paper and the lane-marking yellow.
func TestCardIsDrawnInTheRunningPalette(t *testing.T) {
	_, picture := render(t, marathon())

	if got := picture.At(0, 0); got != asphalt {
		t.Errorf("corner = %v, want the asphalt background %v", got, asphalt)
	}
	if !hasColor(picture, mark) {
		t.Error("the card carries no lane-marking yellow")
	}
	if coverage := ink(picture); coverage < 10000 {
		t.Errorf("only %d pixels differ from the background; the card looks empty", coverage)
	}
}

// Every part of the plan has to reach the picture: two different plans, and the
// same plan in two languages, cannot come out as the same image.
func TestCardShowsThePlanItWasGiven(t *testing.T) {
	marathonRu, _ := render(t, marathon())

	other := marathon()
	other.Time += time.Minute
	slower, _ := render(t, other)
	if bytes.Equal(marathonRu, slower) {
		t.Error("a minute slower renders the same picture")
	}

	shorter := marathon()
	shorter.Distance = 21097
	half, _ := render(t, shorter)
	if bytes.Equal(marathonRu, half) {
		t.Error("another distance renders the same picture")
	}

	english := marathon()
	english.Language = "en"
	inEnglish, _ := render(t, english)
	if bytes.Equal(marathonRu, inEnglish) {
		t.Error("English renders the same picture as Russian")
	}
}

// The same plan always renders the same bytes, so a card can be cached and
// compared without surprises.
func TestRenderingIsDeterministic(t *testing.T) {
	first, _ := render(t, marathon())
	second, _ := render(t, marathon())

	if !bytes.Equal(first, second) {
		t.Error("two renderings of one plan differ")
	}
}

// A picture earns its place by carrying what the sentence cannot: the splits.
// A race too short to split leaves that half of the card empty.
func TestCardShowsSplitsForALongRace(t *testing.T) {
	rightHalf := image.Rect(Width/2, 0, Width, Height)

	_, long := render(t, marathon())
	if coverage := inkIn(long, rightHalf); coverage < 3000 {
		t.Errorf("the splits half has %d pixels of ink; it looks empty", coverage)
	}

	_, short := render(t, Plan{Distance: 1000, Time: 4 * time.Minute, Language: "ru"})
	if coverage := inkIn(short, rightHalf); coverage != 0 {
		t.Errorf("a 1 km plan drew %d pixels of splits; there is nothing to split", coverage)
	}
}

// The caption says the same plan in words, for a reader who cannot see the
// picture.
func TestCaptionSaysThePlanInWords(t *testing.T) {
	if got, want := Caption(marathon()), "Марафон за 3:44:20 — это 5:19 на километр"; got != want {
		t.Errorf("caption = %q, want %q", got, want)
	}

	english := marathon()
	english.Language = "en"
	if got, want := Caption(english), "Marathon in 3:44:20 — that's 5:19 per kilometer"; got != want {
		t.Errorf("caption = %q, want %q", got, want)
	}
}

func TestRenderRejectsAPlanItCannotDraw(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
	}{
		{name: "no distance", plan: Plan{Time: time.Hour, Language: "ru"}},
		{name: "negative distance", plan: Plan{Distance: -1, Time: time.Hour, Language: "ru"}},
		{name: "no time", plan: Plan{Distance: 42195, Language: "ru"}},
		{name: "negative time", plan: Plan{Distance: 42195, Time: -time.Hour, Language: "ru"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Render(tt.plan); err == nil {
				t.Errorf("Render(%+v) = nil error, want a refusal", tt.plan)
			}
		})
	}
}

// An unknown language falls back rather than failing: the card is a picture,
// not an API.
func TestUnknownLanguageStillRenders(t *testing.T) {
	plan := marathon()
	plan.Language = "xx"

	if _, err := Render(plan); err != nil {
		t.Errorf("Render with an unknown language: %v", err)
	}
}
