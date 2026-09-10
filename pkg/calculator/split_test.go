package calculator

import (
	"testing"
	"time"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name        string
		duration    time.Duration
		wantHours   int
		wantMinutes int
		wantSeconds int
	}{
		{
			name:        "under an hour",
			duration:    50 * time.Minute,
			wantMinutes: 50,
		},
		{
			name:        "half marathon",
			duration:    1*time.Hour + 38*time.Minute + 48*time.Second,
			wantHours:   1,
			wantMinutes: 38,
			wantSeconds: 48,
		},
		{
			name:        "sub-minute",
			duration:    45 * time.Second,
			wantSeconds: 45,
		},
		{
			// A 200 km ultra at a walking pace runs past the 60 hour mark;
			// hours must keep counting instead of wrapping around.
			name:        "beyond sixty hours",
			duration:    61*time.Hour + 2*time.Minute + 3*time.Second,
			wantHours:   61,
			wantMinutes: 2,
			wantSeconds: 3,
		},
		{
			name:     "zero",
			duration: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hours, minutes, seconds := Split(tt.duration)

			if hours != tt.wantHours || minutes != tt.wantMinutes || seconds != tt.wantSeconds {
				t.Errorf(
					"Split(%v) = (%d, %d, %d), want (%d, %d, %d)",
					tt.duration, hours, minutes, seconds,
					tt.wantHours, tt.wantMinutes, tt.wantSeconds,
				)
			}
		})
	}
}
