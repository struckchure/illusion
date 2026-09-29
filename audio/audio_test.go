package audio

import (
	"math"
	"testing"
)

func TestTone(t *testing.T) {
	s := Tone(440, 0.1)
	if len(s) != SampleRate/10 {
		t.Fatalf("expected %d samples, got %d", SampleRate/10, len(s))
	}
	if s[0] != 0 || math.Abs(float64(s[len(s)-1])) > 0.01 {
		t.Fatalf("tone should start silent and fade out: first %v last %v", s[0], s[len(s)-1])
	}
	peak := float32(0)
	for _, v := range s {
		peak = max(peak, v)
	}
	if peak < 0.3 || peak > 0.5 {
		t.Fatalf("peak %v", peak)
	}
}
