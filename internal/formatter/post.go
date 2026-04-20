package formatter

import (
	"fmt"
	"strings"

	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// Post holds the text and optional horoscope for image generation.
type Post struct {
	Text          string
	HoroscopeText string // full text for image card, empty if unavailable
	Prediction    prediction.Prediction
	Game         *mlb.Game
}

type GameDayPost struct {
	Game       *mlb.Game
	Record     *mlb.TeamRecord
	H2H        *mlb.H2HRecord
	Pitcher    *mlb.PitcherStats
	Horoscope  *horoscope.Horoscope
	Prediction prediction.Prediction
}

type OffDayPost struct {
	Record    *mlb.TeamRecord
	Horoscope *horoscope.Horoscope
}

func FormatGameDay(p GameDayPost) Post {
	var b strings.Builder

	opponent := p.Game.Opponent().Name
	location := "vs"
	if !p.Game.IsHome {
		location = "@"
	}
	fmt.Fprintf(&b, "⚾ Rockies %s %s\n", location, opponent)
	fmt.Fprintf(&b, "🕐 %s at %s\n", p.Game.FormatGameTime(), p.Game.Venue)

	if p.Pitcher != nil && p.Pitcher.GamesStarted > 0 {
		fmt.Fprintf(&b, "🪖 %s (%.2f ERA, %d-%d)\n", p.Pitcher.FullName, p.Pitcher.ERA, p.Pitcher.Wins, p.Pitcher.Losses)
	}

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
	fmt.Fprintf(&b, "🔮 %s", p.Prediction.FormatPrediction())

	post := Post{
		Text:       strings.TrimSpace(b.String()),
		Prediction: p.Prediction,
		Game:      p.Game,
	}

	if p.Horoscope != nil {
		post.HoroscopeText = p.Horoscope.Text
	}

	return post
}

func FormatOffDay(p OffDayPost) Post {
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

	post := Post{Text: strings.TrimSpace(b.String())}

	if p.Horoscope != nil {
		post.HoroscopeText = p.Horoscope.Text
	}

	return post
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
