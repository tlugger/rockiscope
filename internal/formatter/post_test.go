package formatter

import (
	"strings"
	"testing"
	"time"

	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

func TestFormatGameDay(t *testing.T) {
	post := GameDayPost{
		Game: &mlb.Game{
			GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC),
			IsHome:       true,
			Venue:        "Coors Field",
			HomeTeam:     mlb.TeamInfo{ID: 115, Name: "Colorado Rockies", Wins: 5, Losses: 6},
			AwayTeam:     mlb.TeamInfo{ID: 117, Name: "Houston Astros", Wins: 7, Losses: 4},
		},
		Record: &mlb.TeamRecord{
			Wins: 5, Losses: 6, WinningPercentage: 0.455,
			StreakCode: "W2",
		},
		H2H: &mlb.H2HRecord{
			OpponentName: "Houston Astros", Wins: 2, Losses: 1, GamesPlayed: 3,
		},
		Pitcher: &mlb.PitcherStats{
			FullName: "Michael Lorenzen", ERA: 9.00, Wins: 0, Losses: 1, GamesStarted: 3,
		},
		Horoscope: &horoscope.Horoscope{
			Sign: "cancer",
			Text: "The stars align for an unexpected victory today. Trust your instincts.",
		},
		Prediction: prediction.Prediction{
			WinProbability: 0.45, Pick: "L", Confidence: "A slight celestial nudge toward",
		},
	}

	result := FormatGameDay(post)

	if !strings.Contains(result, "Rockies vs Houston Astros") {
		t.Errorf("missing matchup, got:\n%s", result)
	}
	if !strings.Contains(result, "Coors Field") {
		t.Errorf("missing venue, got:\n%s", result)
	}
	if !strings.Contains(result, "Michael Lorenzen") {
		t.Errorf("missing pitcher, got:\n%s", result)
	}
	if !strings.Contains(result, "9.00 ERA") {
		t.Errorf("missing ERA, got:\n%s", result)
	}
	if !strings.Contains(result, "5-6") {
		t.Errorf("missing record, got:\n%s", result)
	}
	if !strings.Contains(result, "vs HOU: 2-1") {
		t.Errorf("missing H2H, got:\n%s", result)
	}
	if !strings.Contains(result, "W2") {
		t.Errorf("missing streak, got:\n%s", result)
	}
	if !strings.Contains(result, "Prediction:") {
		t.Errorf("missing prediction, got:\n%s", result)
	}
	if len(result) > maxBlueskyChars {
		t.Errorf("post too long: %d chars (max %d)\n%s", len(result), maxBlueskyChars, result)
	}
}

func TestFormatGameDay_Away(t *testing.T) {
	post := GameDayPost{
		Game: &mlb.Game{
			GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC),
			IsHome:       false,
			Venue:        "Minute Maid Park",
			HomeTeam:     mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
			AwayTeam:     mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
		},
		Prediction: prediction.Prediction{Pick: "W", Confidence: "A cosmic coin flip"},
	}

	result := FormatGameDay(post)
	if !strings.Contains(result, "Rockies @ Houston Astros") {
		t.Errorf("expected away format, got:\n%s", result)
	}
}

func TestFormatGameDay_CharLimit(t *testing.T) {
	post := GameDayPost{
		Game: &mlb.Game{
			GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC),
			IsHome:       true,
			Venue:        "Coors Field",
			HomeTeam:     mlb.TeamInfo{ID: 115, Name: "Colorado Rockies", Wins: 50, Losses: 60},
			AwayTeam:     mlb.TeamInfo{ID: 117, Name: "Houston Astros", Wins: 70, Losses: 40},
		},
		Record: &mlb.TeamRecord{Wins: 50, Losses: 60, StreakCode: "L3"},
		H2H:    &mlb.H2HRecord{OpponentName: "Houston Astros", Wins: 3, Losses: 5, GamesPlayed: 8},
		Pitcher: &mlb.PitcherStats{
			FullName: "Michael Lorenzen", ERA: 9.00, Wins: 0, Losses: 1, GamesStarted: 3,
		},
		Horoscope: &horoscope.Horoscope{
			Sign: "cancer",
			Text: strings.Repeat("This is a very long horoscope that should be truncated to fit within the character limit. ", 10),
		},
		Prediction: prediction.Prediction{
			WinProbability: 0.35, Pick: "L", Confidence: "The stars lean toward",
		},
	}

	result := FormatGameDay(post)
	if len(result) > maxBlueskyChars {
		t.Errorf("post too long: %d chars (max %d)\n%s", len(result), maxBlueskyChars, result)
	}
}

func TestFormatOffDay(t *testing.T) {
	post := OffDayPost{
		Record: &mlb.TeamRecord{
			Wins: 5, Losses: 6, WinningPercentage: 0.455,
			RunDifferential: -3, StreakCode: "L1",
		},
		Horoscope: &horoscope.Horoscope{
			Sign: "cancer",
			Text: "Rest and reflection bring clarity today.",
		},
	}

	result := FormatOffDay(post)

	if !strings.Contains(result, "No Rockies game today") {
		t.Errorf("missing off-day header, got:\n%s", result)
	}
	if !strings.Contains(result, "5-6") {
		t.Errorf("missing record, got:\n%s", result)
	}
	if !strings.Contains(result, "Run Diff: -3") {
		t.Errorf("missing run diff, got:\n%s", result)
	}
	if !strings.Contains(result, "Rest and reflection") {
		t.Errorf("missing horoscope, got:\n%s", result)
	}
	if len(result) > maxBlueskyChars {
		t.Errorf("post too long: %d chars (max %d)", len(result), maxBlueskyChars)
	}
}

func TestFormatOffDay_PositiveRunDiff(t *testing.T) {
	post := OffDayPost{
		Record: &mlb.TeamRecord{
			Wins: 10, Losses: 5, WinningPercentage: 0.667,
			RunDifferential: 15,
		},
	}

	result := FormatOffDay(post)
	if !strings.Contains(result, "Run Diff: +15") {
		t.Errorf("expected positive run diff format, got:\n%s", result)
	}
}

func TestShortName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Houston Astros", "HOU"},
		{"Colorado Rockies", "COL"},
		{"Los Angeles Dodgers", "LAD"},
		{"Unknown Team", "UNK"},
	}

	for _, tt := range tests {
		got := shortName(tt.name)
		if got != tt.want {
			t.Errorf("shortName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
