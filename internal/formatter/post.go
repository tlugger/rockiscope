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
	// Starters behind the prediction, for the history record (team = the side
	// being predicted: the Rockies, or the adopted postseason team).
	TeamStarter *prediction.Starter
	OppStarter  *prediction.Starter
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

// FormatGameDay is the pregame post. Its first line ("⚾ Rockies vs X") and
// 🔮 line ("... a Rockies defeat (55%)") are what the Bluesky backfill parses,
// so the voice lives everywhere else.
func FormatGameDay(p GameDayPost) Post {
	seed := seedOf(p.Game.GamePk, p.Game.GameDateTime.Unix())
	horoText := ""
	if p.Horoscope != nil {
		horoText = p.Horoscope.Text
	}

	text := fitting([]string{"kicker", "roast", "pitcher", "record", "venue"}, func(on map[string]bool) string {
		var b strings.Builder
		location := "vs"
		if !p.Game.IsHome {
			location = "@"
		}
		fmt.Fprintf(&b, "⚾ Rockies %s %s\n", location, p.Game.Opponent().Name)

		venue := fmt.Sprintf("🕐 %s at %s", p.Game.FormatGameTime(), p.Game.Venue)
		if on["venue"] {
			quips := awayVenueQuips
			if p.Game.IsHome {
				quips = homeVenueQuips
			}
			venue = withQuip(venue+".", pick(quips, seed))
		}
		b.WriteString(venue + "\n")

		if p.Pitcher != nil && p.Pitcher.GamesStarted > 0 {
			line := fmt.Sprintf("🪖 %s (%.2f ERA, %d-%d)", p.Pitcher.FullName, p.Pitcher.ERA, p.Pitcher.Wins, p.Pitcher.Losses)
			if q := eraQuip(p.Pitcher.ERA, seed); on["pitcher"] && q != "" {
				line += ". " + q
			}
			b.WriteString(line + "\n")
		}

		if p.Record != nil {
			line := fmt.Sprintf("📊 %d-%d", p.Record.Wins, p.Record.Losses)
			if p.H2H != nil && p.H2H.GamesPlayed > 0 {
				line += fmt.Sprintf(" | vs %s: %d-%d", shortName(p.H2H.OpponentName), p.H2H.Wins, p.H2H.Losses)
			}
			if p.Record.StreakCode != "" {
				line += " | " + p.Record.StreakCode
			}
			if q := streakQuip(p.Record.StreakCode, seed); on["record"] && q != "" {
				line += ". " + q
			}
			b.WriteString(line + "\n")
		}

		if r := horoscopeRoast(horoText, seed, roastGameDay); on["roast"] && r != "" {
			b.WriteString(r + "\n")
		}

		fmt.Fprintf(&b, "\n🔮 %s", p.Prediction.FormatPrediction())
		if on["kicker"] {
			b.WriteString("\n" + predictionKicker(p.Prediction.WinProbability, seed))
		}
		return b.String()
	})

	return Post{Text: text, Prediction: p.Prediction, Game: p.Game, HoroscopeText: horoText}
}

var offDayOpeners = []string{
	"The stars are resting. So is the bullpen.",
	"Nobody can lose today. Savor it.",
	"Undefeated today, mathematically.",
	"A perfect day: zero losses.",
	"The cosmos have granted a reprieve.",
	"No baseball. Just vibes and regret.",
}

// FormatOffDay keeps its "⚾ No Rockies game today" opener for the backfill.
func FormatOffDay(p OffDayPost) Post {
	horoText := ""
	if p.Horoscope != nil {
		horoText = p.Horoscope.Text
	}
	seed := seedOf("offday", horoText)
	if p.Record != nil {
		seed = seedOf("offday", horoText, p.Record.Wins, p.Record.Losses)
	}

	text := fitting([]string{"roast", "opener", "record"}, func(on map[string]bool) string {
		var b strings.Builder
		b.WriteString("⚾ No Rockies game today.")
		if on["opener"] {
			b.WriteString(" " + pick(offDayOpeners, seed))
		}
		b.WriteString("\n")

		if p.Record != nil {
			line := fmt.Sprintf("📊 %d-%d (%.3f)", p.Record.Wins, p.Record.Losses, p.Record.WinningPercentage)
			if p.Record.RunDifferential >= 0 {
				line += fmt.Sprintf(" | Run Diff: +%d", p.Record.RunDifferential)
			} else {
				line += fmt.Sprintf(" | Run Diff: %d", p.Record.RunDifferential)
			}
			if p.Record.StreakCode != "" {
				line += " | " + p.Record.StreakCode
			}
			q := runDiffQuip(p.Record.RunDifferential, seed)
			if q == "" {
				q = streakQuip(p.Record.StreakCode, seed)
			}
			if on["record"] && q != "" {
				line += ". " + q
			}
			b.WriteString(line + "\n")
		}

		if r := horoscopeRoast(horoText, seed, roastOffDay); on["roast"] && r != "" {
			b.WriteString("\n" + r)
		}
		return strings.TrimSpace(b.String())
	})

	return Post{Text: text, HoroscopeText: horoText}
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
