package formatter

import (
	"fmt"
	"strings"

	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// ThreadPost holds the main post and an optional reply for threading.
type ThreadPost struct {
	Main  string
	Reply string // empty if no horoscope available
}

// GameDayPost builds the post text for a game day.
type GameDayPost struct {
	Game       *mlb.Game
	Record     *mlb.TeamRecord
	H2H        *mlb.H2HRecord
	Pitcher    *mlb.PitcherStats
	Horoscope  *horoscope.Horoscope
	Prediction prediction.Prediction
}

// OffDayPost builds the post text for an off day.
type OffDayPost struct {
	Record    *mlb.TeamRecord
	Horoscope *horoscope.Horoscope
}

// FormatGameDay produces the game day thread.
func FormatGameDay(p GameDayPost) ThreadPost {
	var b strings.Builder

	// Matchup
	opponent := p.Game.Opponent().Name
	location := "vs"
	if !p.Game.IsHome {
		location = "@"
	}
	fmt.Fprintf(&b, "⚾ Rockies %s %s\n", location, opponent)

	// Game info
	fmt.Fprintf(&b, "🕐 %s at %s\n", p.Game.FormatGameTime(), p.Game.Venue)

	// Pitcher
	if p.Pitcher != nil && p.Pitcher.GamesStarted > 0 {
		fmt.Fprintf(&b, "🪖 %s (%.2f ERA, %d-%d)\n", p.Pitcher.FullName, p.Pitcher.ERA, p.Pitcher.Wins, p.Pitcher.Losses)
	}

	// Record + H2H
	if p.Record != nil {
		fmt.Fprintf(&b, "📊 %d-%d", p.Record.Wins, p.Record.Losses)
		if p.H2H != nil && p.H2H.GamesPlayed > 0 {
			fmt.Fprintf(&b, " | vs %s: %d-%d", shortName(p.H2H.OpponentName), p.H2H.Wins, p.H2H.Losses)
		}
		if p.Record.StreakCode != "" {
			fmt.Fprintf(&b, " | %s", p.Record.StreakCode)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Prediction
	fmt.Fprintf(&b, "🔮 %s", p.Prediction.FormatPrediction())

	thread := ThreadPost{Main: strings.TrimSpace(b.String())}

	// Horoscope as reply
	if p.Horoscope != nil {
		thread.Reply = fmt.Sprintf("♋ Today's Cancer horoscope:\n\n%s", p.Horoscope.Text)
	}

	return thread
}

// FormatOffDay produces the off day thread.
func FormatOffDay(p OffDayPost) ThreadPost {
	var b strings.Builder

	b.WriteString("⚾ No Rockies game today.\n")

	if p.Record != nil {
		fmt.Fprintf(&b, "📊 %d-%d (%.3f)", p.Record.Wins, p.Record.Losses, p.Record.WinningPercentage)
		if p.Record.RunDifferential >= 0 {
			fmt.Fprintf(&b, " | Run Diff: +%d", p.Record.RunDifferential)
		} else {
			fmt.Fprintf(&b, " | Run Diff: %d", p.Record.RunDifferential)
		}
		if p.Record.StreakCode != "" {
			fmt.Fprintf(&b, " | %s", p.Record.StreakCode)
		}
		b.WriteString("\n")
	}

	thread := ThreadPost{Main: strings.TrimSpace(b.String())}

	if p.Horoscope != nil {
		thread.Reply = fmt.Sprintf("♋ Today's Cancer horoscope:\n\n%s", p.Horoscope.Text)
	}

	return thread
}

func shortName(name string) string {
	abbrevs := map[string]string{
		"Arizona Diamondbacks":  "ARI", "Atlanta Braves": "ATL",
		"Baltimore Orioles":     "BAL", "Boston Red Sox": "BOS",
		"Chicago Cubs":          "CHC", "Chicago White Sox": "CHW",
		"Cincinnati Reds":       "CIN", "Cleveland Guardians": "CLE",
		"Colorado Rockies":      "COL", "Detroit Tigers": "DET",
		"Houston Astros":        "HOU", "Kansas City Royals": "KC",
		"Los Angeles Angels":    "LAA", "Los Angeles Dodgers": "LAD",
		"Miami Marlins":         "MIA", "Milwaukee Brewers": "MIL",
		"Minnesota Twins":       "MIN", "New York Mets": "NYM",
		"New York Yankees":      "NYY", "Oakland Athletics": "OAK",
		"Philadelphia Phillies": "PHI", "Pittsburgh Pirates": "PIT",
		"San Diego Padres":      "SD", "San Francisco Giants": "SF",
		"Seattle Mariners":      "SEA", "St. Louis Cardinals": "STL",
		"Tampa Bay Rays":        "TB", "Texas Rangers": "TEX",
		"Toronto Blue Jays":     "TOR", "Washington Nationals": "WSH",
	}
	if abbr, ok := abbrevs[name]; ok {
		return abbr
	}
	if len(name) >= 3 {
		return strings.ToUpper(name[:3])
	}
	return name
}
