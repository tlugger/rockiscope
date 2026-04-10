package image

import (
	"os"
	"testing"
)

func TestHoroscopeCard(t *testing.T) {
	text := "Cancer, you're likely to want to retreat into your bedroom and slam the door today. You won't feel like talking or socializing with anyone, not even those closest to you. Too much work could have you in a state of near exhaustion and almost total burnout, which means that getting some rest is probably the best thing you could do right now."

	pngBytes, w, h, err := HoroscopeCard(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w != 800 {
		t.Errorf("width = %d, want 800", w)
	}
	if h < 150 || h > 600 {
		t.Errorf("height = %d, expected between 150-600", h)
	}
	if len(pngBytes) < 1000 {
		t.Errorf("PNG too small: %d bytes", len(pngBytes))
	}
	if len(pngBytes) > 1_000_000 {
		t.Errorf("PNG too large for Bluesky: %d bytes (max 1MB)", len(pngBytes))
	}
}

func TestHoroscopeCard_Short(t *testing.T) {
	pngBytes, _, _, err := HoroscopeCard("Good vibes today.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pngBytes) < 100 {
		t.Error("expected valid PNG output")
	}
}

// TestHoroscopeCard_Sample generates a sample card to disk for visual inspection.
// Run with: go test -run TestHoroscopeCard_Sample -v
func TestHoroscopeCard_Sample(t *testing.T) {
	if os.Getenv("GENERATE_SAMPLE") == "" {
		t.Skip("set GENERATE_SAMPLE=1 to run")
	}

	text := "Cancer, you're likely to want to retreat into your bedroom and slam the door today. You won't feel like talking or socializing with anyone, not even those closest to you. Too much work could have you in a state of near exhaustion and almost total burnout, which means that getting some rest is probably the best thing you could do right now. Relax now, and get yourself going again tomorrow."

	pngBytes, _, _, err := HoroscopeCard(text)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	path := "../../sample_card.png"
	if err := os.WriteFile(path, pngBytes, 0644); err != nil {
		t.Fatalf("writing sample: %v", err)
	}
	t.Logf("wrote sample card to %s (%d bytes)", path, len(pngBytes))
}
