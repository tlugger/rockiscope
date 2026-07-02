package bluesky

import (
	"math"
	"testing"
)

func TestParsePredictionPost_GameDay(t *testing.T) {
	text := `⚾ Rockies vs Miami Marlins
🕐 6:40 PM MDT at Coors Field
🪖 Kyle Freeland (7.50 ERA, 1-7)
📊 33-53 | vs MIA: 0-5 | L3

🔮 The stars lean toward a Rockies defeat (66%)`

	pp := ParsePredictionPost(text, "2026-07-01")
	if pp == nil {
		t.Fatal("expected parsed prediction, got nil")
	}
	if pp.Opponent != "Miami Marlins" {
		t.Errorf("opponent = %q, want %q", pp.Opponent, "Miami Marlins")
	}
	if pp.Predicted != "L" {
		t.Errorf("predicted = %q, want %q", pp.Predicted, "L")
	}
	if math.Abs(pp.WinProb-0.66) > 0.001 {
		t.Errorf("winProb = %f, want %f", pp.WinProb, 0.66)
	}
	if pp.Date != "2026-07-01" {
		t.Errorf("date = %q, want %q", pp.Date, "2026-07-01")
	}
}

func TestParsePredictionPost_AwayGame(t *testing.T) {
	text := `⚾ Rockies @ San Diego Padres
🕐 1:40 PM MDT at Petco Park
🪖 Austin Gomber (4.50 ERA, 2-3)

🔮 The cosmos strongly favor a Rockies victory (73%)`

	pp := ParsePredictionPost(text, "2026-04-09")
	if pp == nil {
		t.Fatal("expected parsed prediction, got nil")
	}
	if pp.Opponent != "San Diego Padres" {
		t.Errorf("opponent = %q, want %q", pp.Opponent, "San Diego Padres")
	}
	if pp.Predicted != "W" {
		t.Errorf("predicted = %q, want %q", pp.Predicted, "W")
	}
	if math.Abs(pp.WinProb-0.73) > 0.001 {
		t.Errorf("winProb = %f, want %f", pp.WinProb, 0.73)
	}
}

func TestParsePredictionPost_NoPitcher(t *testing.T) {
	text := `⚾ Rockies vs Atlanta Braves
🕐 6:40 PM MDT at Coors Field

🔮 A cosmic coin flip a Rockies victory (50%)`

	pp := ParsePredictionPost(text, "2026-05-01")
	if pp == nil {
		t.Fatal("expected parsed prediction, got nil")
	}
	if pp.Opponent != "Atlanta Braves" {
		t.Errorf("opponent = %q, want %q", pp.Opponent, "Atlanta Braves")
	}
	if pp.Predicted != "W" {
		t.Errorf("predicted = %q, want %q", pp.Predicted, "W")
	}
	if math.Abs(pp.WinProb-0.50) > 0.001 {
		t.Errorf("winProb = %f, want %f", pp.WinProb, 0.50)
	}
}

func TestParsePredictionPost_OffDay(t *testing.T) {
	text := `⚾ No Rockies game today.
📊 33-53 (0.384) | Run Diff: -42 | L2`

	pp := ParsePredictionPost(text, "2026-07-02")
	if pp != nil {
		t.Errorf("expected nil for off-day, got %+v", pp)
	}
}

func TestParsePredictionPost_Reply(t *testing.T) {
	text := `📚 Another chapter in the prophecy fulfilled.

🏁 Rockies L
📊 Colorado Rockies 3-14 Miami Marlins
🎯 24/44 correct predictions this season`

	pp := ParsePredictionPost(text, "2026-07-01")
	if pp != nil {
		t.Errorf("expected nil for reply, got %+v", pp)
	}
}

func TestParsePredictionPost_EmptyText(t *testing.T) {
	pp := ParsePredictionPost("", "2026-07-01")
	if pp != nil {
		t.Errorf("expected nil for empty text, got %+v", pp)
	}
}

func TestParsePredictionPost_NudgeVictory(t *testing.T) {
	text := `⚾ Rockies vs Los Angeles Dodgers
🕐 1:10 PM MDT at Coors Field

🔮 A slight celestial nudge toward a Rockies victory (53%)`

	pp := ParsePredictionPost(text, "2026-04-17")
	if pp == nil {
		t.Fatal("expected parsed prediction, got nil")
	}
	if pp.Opponent != "Los Angeles Dodgers" {
		t.Errorf("opponent = %q, want %q", pp.Opponent, "Los Angeles Dodgers")
	}
	if pp.Predicted != "W" {
		t.Errorf("predicted = %q, want %q", pp.Predicted, "W")
	}
	if math.Abs(pp.WinProb-0.53) > 0.001 {
		t.Errorf("winProb = %f, want %f", pp.WinProb, 0.53)
	}
}
