package card

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWriteSampleCards is a tool for looking at the drawing, not a check:
// CARD_SAMPLE_DIR=/tmp go test ./pkg/card -run TestWriteSampleCards
func TestWriteSampleCards(t *testing.T) {
	dir := os.Getenv("CARD_SAMPLE_DIR")
	if dir == "" {
		t.Skip("set CARD_SAMPLE_DIR to write sample cards")
	}

	plans := []Plan{
		{Distance: 42195, Time: 3*time.Hour + 44*time.Minute + 20*time.Second, Language: "ru"},
		{Distance: 21097, Time: time.Hour + 38*time.Minute + 48*time.Second, Language: "en"},
		{Distance: 10000, Time: 50 * time.Minute, Language: "ru"},
		{Distance: 5500, Time: 27*time.Minute + 30*time.Second, Language: "en"},
	}

	for _, plan := range plans {
		data, err := Render(plan)
		if err != nil {
			t.Fatalf("render %+v: %v", plan, err)
		}

		name := filepath.Join(dir, fmt.Sprintf("card-%d-%s.png", plan.Distance, plan.Language))
		if err := os.WriteFile(name, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		t.Log("wrote", name)
	}
}
