package formatter

import (
	"fmt"
	"strings"

	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

const maxBlueskyChars = 300

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

// FormatGameDay produces the game day post text, fitting within Bluesky's limit.
func FormatGameDay(p GameDayPost) string {
	var b strings.Builder

	// Line 1: Matchup
	opponent := p.Game.Opponent().Name
	location := "vs"
	if !p.Game.IsHome {
		location = "@"
	}
	fmt.Fprintf(&b, "%s %s %s\n", "Rockies", location, opponent)

	// Line 2: Game info
	fmt.Fprintf(&b, "%s at %s\n", p.Game.FormatGameTime(), p.Game.Venue)

	// Line 3: Pitcher (if available)
	if p.Pitcher != nil && p.Pitcher.GamesStarted > 0 {
		fmt.Fprintf(&b, "SP: %s (%.2f ERA, %d-%d)\n", p.Pitcher.FullName, p.Pitcher.ERA, p.Pitcher.Wins, p.Pitcher.Losses)
	}

	// Line 4: Record + H2H
	if p.Record != nil {
		fmt.Fprintf(&b, "Season: %d-%d", p.Record.Wins, p.Record.Losses)
		if p.H2H != nil && p.H2H.GamesPlayed > 0 {
			fmt.Fprintf(&b, " | vs %s: %d-%d", shortName(p.H2H.OpponentName), p.H2H.Wins, p.H2H.Losses)
		}
		if p.Record.StreakCode != "" {
			fmt.Fprintf(&b, " | %s", p.Record.StreakCode)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Prediction line
	fmt.Fprintf(&b, "Prediction: %s\n", p.Prediction.FormatPrediction())

	b.WriteString("\n")

	// Horoscope — fill remaining space
	if p.Horoscope != nil {
		remaining := maxBlueskyChars - b.Len() - 2 // leave room for newline
		if remaining > 30 {
			horoText := horoscope.Truncate(p.Horoscope.Text, remaining)
			fmt.Fprintf(&b, "%s", horoText)
		}
	}

	return strings.TrimSpace(b.String())
}

// FormatOffDay produces the off day post text.
func FormatOffDay(p OffDayPost) string {
	var b strings.Builder

	b.WriteString("No Rockies game today.\n")

	if p.Record != nil {
		fmt.Fprintf(&b, "Season: %d-%d (%.3f)", p.Record.Wins, p.Record.Losses, p.Record.WinningPercentage)
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

	b.WriteString("\n")

	// Horoscope fills remaining space
	if p.Horoscope != nil {
		remaining := maxBlueskyChars - b.Len() - 2
		if remaining > 30 {
			horoText := horoscope.Truncate(p.Horoscope.Text, remaining)
			fmt.Fprintf(&b, "%s", horoText)
		}
	}

	return strings.TrimSpace(b.String())
}

// shortName trims a team name to its city/nickname for brevity.
// "Houston Astros" -> "HOU", "Colorado Rockies" -> "COL"
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
	// Fallback: use first 3 chars
	if len(name) >= 3 {
		return strings.ToUpper(name[:3])
	}
	return name
}
