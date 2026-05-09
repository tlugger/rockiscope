package prediction

import (
	"math"
	"testing"

	"github.com/tlugger/rockiscope/internal/mlb"
)

func TestPredict_FullData(t *testing.T) {
	in := Input{
		Record: &mlb.TeamRecord{
			Wins: 60, Losses: 40, WinningPercentage: 0.600,
			HomeWins: 35, HomeLosses: 15,
			AwayWins: 25, AwayLosses: 25,
			StreakCode: "W3",
		},
		RockiesPitcher:  &mlb.PitcherStats{ERA: 3.50, WHIP: 1.20, InningsPitched: 100},
		OpponentPitcher: &mlb.PitcherStats{ERA: 4.80, WHIP: 1.45, InningsPitched: 90},
		HeadToHead:      &mlb.H2HRecord{Wins: 4, Losses: 2, GamesPlayed: 6},
		IsHome:          true,
		HoroscopeText:   "The stars align for great fortune today",
	}

	p := Predict(in, DefaultWeights())

	if p.Pick != "W" {
		t.Errorf("expected W with favorable stats, got %s", p.Pick)
	}
	if p.WinProbability < 0.55 {
		t.Errorf("expected probability > 0.55, got %f", p.WinProbability)
	}
	if p.Confidence == "" {
		t.Error("expected non-empty confidence label")
	}
}

func TestPredict_Underdog(t *testing.T) {
	in := Input{
		Record: &mlb.TeamRecord{
			Wins: 30, Losses: 70, WinningPercentage: 0.300,
			HomeWins: 20, HomeLosses: 30,
			AwayWins: 10, AwayLosses: 40,
			StreakCode: "L5",
		},
		RockiesPitcher:  &mlb.PitcherStats{ERA: 6.50, WHIP: 1.90, InningsPitched: 80},
		OpponentPitcher: &mlb.PitcherStats{ERA: 2.50, WHIP: 0.95, InningsPitched: 120},
		HeadToHead:      &mlb.H2HRecord{Wins: 1, Losses: 5, GamesPlayed: 6},
		IsHome:          false,
		HoroscopeText:   "Today brings challenges and tests of patience",
	}

	p := Predict(in, DefaultWeights())

	if p.Pick != "L" {
		t.Errorf("expected L with poor stats, got %s (prob=%f)", p.Pick, p.WinProbability)
	}
	if p.WinProbability > 0.45 {
		t.Errorf("expected probability < 0.45, got %f", p.WinProbability)
	}
}

func TestPredict_NoData(t *testing.T) {
	p := Predict(Input{}, DefaultWeights())

	if p.WinProbability < 0.05 || p.WinProbability > 0.95 {
		t.Errorf("probability out of bounds: %f", p.WinProbability)
	}
	if p.Pick != "W" && p.Pick != "L" {
		t.Errorf("unexpected pick: %s", p.Pick)
	}
}

func TestPredict_OnlyHoroscope(t *testing.T) {
	p := Predict(Input{HoroscopeText: "A glorious day of cosmic triumph"}, DefaultWeights())

	if p.WinProbability < 0.05 || p.WinProbability > 0.95 {
		t.Errorf("probability out of bounds: %f", p.WinProbability)
	}
}

func TestPredict_EarlySeason(t *testing.T) {
	in := Input{
		Record: &mlb.TeamRecord{
			Wins: 2, Losses: 1, WinningPercentage: 0.667,
			StreakCode: "W2",
		},
		IsHome:        true,
		HoroscopeText: "New beginnings favor the bold",
	}

	p := Predict(in, DefaultWeights())

	// Should still produce a valid prediction
	if p.WinProbability < 0.05 || p.WinProbability > 0.95 {
		t.Errorf("probability out of bounds: %f", p.WinProbability)
	}
}

func TestPredict_Clamped(t *testing.T) {
	// Even with extreme data, should clamp to [0.05, 0.95]
	in := Input{
		Record: &mlb.TeamRecord{
			Wins: 100, Losses: 0, WinningPercentage: 1.0,
			HomeWins: 50, HomeLosses: 0,
			StreakCode: "W10",
		},
		RockiesPitcher:  &mlb.PitcherStats{ERA: 0.50, WHIP: 0.50, InningsPitched: 200},
		OpponentPitcher: &mlb.PitcherStats{ERA: 9.00, WHIP: 3.00, InningsPitched: 100},
		HeadToHead:      &mlb.H2HRecord{Wins: 10, Losses: 0, GamesPlayed: 10},
		IsHome:          true,
		HoroscopeText:   "Absolute perfection awaits",
	}

	p := Predict(in, DefaultWeights())
	if p.WinProbability > 0.95 {
		t.Errorf("probability should be clamped to 0.95, got %f", p.WinProbability)
	}
}

func TestPitcherFactor(t *testing.T) {
	tests := []struct {
		name     string
		rockies  *mlb.PitcherStats
		opponent *mlb.PitcherStats
		wantMin  float64
		wantMax  float64
	}{
		{
			name:     "nil pitchers",
			rockies:  nil,
			opponent: nil,
			wantMin:  -1, wantMax: -1,
		},
		{
			name:     "insufficient innings",
			rockies:  &mlb.PitcherStats{ERA: 3.0, InningsPitched: 2},
			opponent: &mlb.PitcherStats{ERA: 4.0, InningsPitched: 100},
			wantMin:  -1, wantMax: -1,
		},
		{
			name:     "rockies ace vs bad pitcher",
			rockies:  &mlb.PitcherStats{ERA: 2.0, WHIP: 0.90, InningsPitched: 100},
			opponent: &mlb.PitcherStats{ERA: 7.0, WHIP: 2.00, InningsPitched: 80},
			wantMin:  0.6, wantMax: 1.0,
		},
		{
			name:     "equal pitchers",
			rockies:  &mlb.PitcherStats{ERA: 4.0, WHIP: 1.30, InningsPitched: 100},
			opponent: &mlb.PitcherStats{ERA: 4.0, WHIP: 1.30, InningsPitched: 100},
			wantMin:  0.45, wantMax: 0.55,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pitcherFactor(tt.rockies, tt.opponent)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("pitcherFactor = %f, want [%f, %f]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestStreakScore(t *testing.T) {
	tests := []struct {
		code string
		want float64
	}{
		{"W3", 0.65},
		{"L3", 0.35},
		{"W10", 1.0},
		{"L10", 0.0},
		{"W1", 0.55},
		{"", 0.5},
	}

	for _, tt := range tests {
		got := streakScore(tt.code)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("streakScore(%q) = %f, want %f", tt.code, got, tt.want)
		}
	}
}

func TestHoroscopeScore_Deterministic(t *testing.T) {
	text := "The stars shine bright for Cancer today"
	s1 := horoscopeScore(text)
	s2 := horoscopeScore(text)
	if s1 != s2 {
		t.Errorf("horoscopeScore not deterministic: %f != %f", s1, s2)
	}
	if s1 < 0 || s1 > 1 {
		t.Errorf("horoscopeScore out of range: %f", s1)
	}
}

func TestHoroscopeScore_Empty(t *testing.T) {
	if horoscopeScore("") != 0.5 {
		t.Errorf("empty text should return 0.5")
	}
}

func TestConfidenceLabel(t *testing.T) {
	tests := []struct {
		prob float64
		want string
	}{
		{0.95, "The stars are screaming"},
		{0.75, "The cosmos strongly favor"},
		{0.70, "The stars lean toward"},
		{0.58, "A slight celestial nudge toward"},
		{0.51, "A cosmic coin flip"},
		{0.50, "A cosmic coin flip"},
	}

	for _, tt := range tests {
		got := confidenceLabel(tt.prob)
		if got != tt.want {
			t.Errorf("confidenceLabel(%f) = %q, want %q", tt.prob, got, tt.want)
		}
	}
}

func TestFormatPrediction(t *testing.T) {
	p := Prediction{WinProbability: 0.72, Pick: "W", Confidence: "The stars lean toward"}
	s := p.FormatPrediction()
	if s != "The stars lean toward a Rockies victory (72%)" {
		t.Errorf("format = %q", s)
	}

	p2 := Prediction{WinProbability: 0.35, Pick: "L", Confidence: "The stars lean toward"}
	s2 := p2.FormatPrediction()
	if s2 != "The stars lean toward a Rockies defeat (65%)" {
		t.Errorf("format = %q", s2)
	}
}
