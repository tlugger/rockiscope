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
		Record:  &mlb.TeamRecord{Wins: 5, Losses: 6, StreakCode: "W2"},
		H2H:     &mlb.H2HRecord{OpponentName: "Houston Astros", Wins: 2, Losses: 1, GamesPlayed: 3},
		Pitcher: &mlb.PitcherStats{FullName: "Michael Lorenzen", ERA: 9.00, Wins: 0, Losses: 1, GamesStarted: 3},
		Horoscope: &horoscope.Horoscope{
			Sign: "cancer",
			Text: "The stars align for an unexpected victory today.",
		},
		Prediction: prediction.Prediction{WinProbability: 0.45, Pick: "L", Confidence: "A slight celestial nudge toward"},
	}

	result := FormatGameDay(post)

	if !strings.Contains(result.Text, "⚾ Rockies vs Houston Astros") {
		t.Errorf("missing matchup:\n%s", result.Text)
	}
	if !strings.Contains(result.Text, "🪖 Michael Lorenzen") {
		t.Errorf("missing pitcher:\n%s", result.Text)
	}
	if !strings.Contains(result.Text, "📊 5-6") {
		t.Errorf("missing record:\n%s", result.Text)
	}
	if !strings.Contains(result.Text, "🔮") {
		t.Errorf("missing prediction:\n%s", result.Text)
	}

	// Horoscope should be separate, not in text
	if strings.Contains(result.Text, "stars align") {
		t.Errorf("horoscope should not be in text:\n%s", result.Text)
	}
	if result.HoroscopeText != "The stars align for an unexpected victory today." {
		t.Errorf("horoscope text = %q", result.HoroscopeText)
	}
}

func TestFormatGameDay_Away(t *testing.T) {
	result := FormatGameDay(GameDayPost{
		Game: &mlb.Game{
			GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC),
			IsHome:       false,
			Venue:        "Minute Maid Park",
			HomeTeam:     mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
			AwayTeam:     mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
		},
		Prediction: prediction.Prediction{Pick: "W", Confidence: "A cosmic coin flip"},
	})

	if !strings.Contains(result.Text, "⚾ Rockies @ Houston Astros") {
		t.Errorf("expected away format:\n%s", result.Text)
	}
}

func TestFormatGameDay_NoHoroscope(t *testing.T) {
	result := FormatGameDay(GameDayPost{
		Game: &mlb.Game{
			GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC),
			IsHome:       true, Venue: "Coors Field",
			HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
			AwayTeam: mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
		},
		Prediction: prediction.Prediction{Pick: "L", Confidence: "A cosmic coin flip"},
	})

	if result.HoroscopeText != "" {
		t.Errorf("expected empty horoscope text, got: %q", result.HoroscopeText)
	}
}

func TestFormatOffDay(t *testing.T) {
	result := FormatOffDay(OffDayPost{
		Record: &mlb.TeamRecord{
			Wins: 5, Losses: 6, WinningPercentage: 0.455,
			RunDifferential: -3, StreakCode: "L1",
		},
		Horoscope: &horoscope.Horoscope{Sign: "cancer", Text: "Rest and reflect."},
	})

	if !strings.Contains(result.Text, "⚾ No Rockies game today") {
		t.Errorf("missing header:\n%s", result.Text)
	}
	if !strings.Contains(result.Text, "Run Diff: -3") {
		t.Errorf("missing run diff:\n%s", result.Text)
	}
	if result.HoroscopeText != "Rest and reflect." {
		t.Errorf("horoscope = %q", result.HoroscopeText)
	}
}

func TestFormatOffDay_PositiveRunDiff(t *testing.T) {
	result := FormatOffDay(OffDayPost{
		Record: &mlb.TeamRecord{Wins: 10, Losses: 5, WinningPercentage: 0.667, RunDifferential: 15},
	})
	if !strings.Contains(result.Text, "Run Diff: +15") {
		t.Errorf("expected positive:\n%s", result.Text)
	}
}

func TestShortName(t *testing.T) {
	tests := []struct{ name, want string }{
		{"Houston Astros", "HOU"},
		{"Colorado Rockies", "COL"},
		{"Los Angeles Dodgers", "LAD"},
		{"Unknown Team", "UNK"},
	}
	for _, tt := range tests {
		if got := shortName(tt.name); got != tt.want {
			t.Errorf("shortName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
