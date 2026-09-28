package httpapi

import (
	"testing"
	"time"
)

// Playback of a recording at a higher speed sends further ahead (the player drains its buffer faster), capped.
func TestPlaybackLead(t *testing.T) {
	for _, c := range []struct {
		speed float64
		want  time.Duration
	}{{0.5, 500 * time.Millisecond}, {1, 500 * time.Millisecond}, {2, time.Second}, {4, 2 * time.Second}, {16, 2 * time.Second}} {
		if got := playbackLead(c.speed); got != c.want {
			t.Errorf("speed %v: lead %v, want %v", c.speed, got, c.want)
		}
	}
}
