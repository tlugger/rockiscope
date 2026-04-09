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
			Text: "The stars align for an unexpected victory today. Trust your instincts and let the cosmic energy guide you through challenges.",
		},
		Prediction: prediction.Prediction{
			WinProbability: 0.45, Pick: "L", Confidence: "A slight celestial nudge toward",
		},
	}

	thread := FormatGameDay(post)

	// Main post checks
	if !strings.Contains(thread.Main, "⚾ Rockies vs Houston Astros") {
		t.Errorf("missing matchup, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "🕐") {
		t.Errorf("missing clock emoji, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "Coors Field") {
		t.Errorf("missing venue, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "🪖 Michael Lorenzen") {
		t.Errorf("missing pitcher, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "📊 5-6") {
		t.Errorf("missing record, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "vs HOU: 2-1") {
		t.Errorf("missing H2H, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "🔮") {
		t.Errorf("missing prediction emoji, got:\n%s", thread.Main)
	}

	// Main post should NOT contain horoscope text
	if strings.Contains(thread.Main, "stars align") {
		t.Errorf("main post should not contain horoscope text:\n%s", thread.Main)
	}

	// Reply should have full horoscope
	if !strings.Contains(thread.Reply, "♋") {
		t.Errorf("missing cancer emoji in reply, got:\n%s", thread.Reply)
	}
	if !strings.Contains(thread.Reply, "stars align for an unexpected victory") {
		t.Errorf("reply should contain full horoscope text:\n%s", thread.Reply)
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

	thread := FormatGameDay(post)
	if !strings.Contains(thread.Main, "⚾ Rockies @ Houston Astros") {
		t.Errorf("expected away format, got:\n%s", thread.Main)
	}
}

func TestFormatGameDay_NoHoroscope(t *testing.T) {
	post := GameDayPost{
		Game: &mlb.Game{
			GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC),
			IsHome:       true,
			Venue:        "Coors Field",
			HomeTeam:     mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
			AwayTeam:     mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
		},
		Prediction: prediction.Prediction{Pick: "L", Confidence: "A cosmic coin flip"},
	}

	thread := FormatGameDay(post)
	if thread.Reply != "" {
		t.Errorf("expected empty reply with no horoscope, got:\n%s", thread.Reply)
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
			Text: "Rest and reflection bring clarity today. The cosmos suggest patience and self-care.",
		},
	}

	thread := FormatOffDay(post)

	if !strings.Contains(thread.Main, "⚾ No Rockies game today") {
		t.Errorf("missing off-day header, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "📊 5-6") {
		t.Errorf("missing record, got:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Main, "Run Diff: -3") {
		t.Errorf("missing run diff, got:\n%s", thread.Main)
	}

	// Horoscope in reply, not main
	if strings.Contains(thread.Main, "Rest and reflection") {
		t.Errorf("main should not have horoscope:\n%s", thread.Main)
	}
	if !strings.Contains(thread.Reply, "Rest and reflection") {
		t.Errorf("reply should have horoscope:\n%s", thread.Reply)
	}
}

func TestFormatOffDay_PositiveRunDiff(t *testing.T) {
	post := OffDayPost{
		Record: &mlb.TeamRecord{
			Wins: 10, Losses: 5, WinningPercentage: 0.667,
			RunDifferential: 15,
		},
	}

	thread := FormatOffDay(post)
	if !strings.Contains(thread.Main, "Run Diff: +15") {
		t.Errorf("expected positive run diff format, got:\n%s", thread.Main)
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
