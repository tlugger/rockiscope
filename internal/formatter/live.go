package formatter

import (
	"fmt"
	"math"
	"strings"

	"github.com/tlugger/rockiscope/internal/astro"
)

// LiveUpdate is one in-game post: a notable play, the stars' read on the
// batter/pitcher matchup, and where that leaves the cosmic win probability.
type LiveUpdate struct {
	Team                  string // nickname of the team we're following
	Inning                int
	IsTop                 bool
	AwayAbbrev            string
	HomeAbbrev            string
	AwayScore             int
	HomeScore             int
	EventType             string
	IsScoringPlay         bool
	Headline              string // the clip's headline, or MLB's play description
	Batter, Pitcher       string
	SignsKnown            bool
	Matchup               astro.Matchup
	Destiny               float64
	HelpedUs              bool
	BatterWon             bool    // the batter's side gained on the play
	WPA                   float64 // our team's win probability added, points
	Leverage              float64
	LaunchSpeed, Distance float64
	Real, Cosmic          float64 // 0-100
	PrevCosmic            float64
	Seed                  int
}

// foreseenAt is the |Destiny| at which the stars claim (or disown) a play.
// Any elemental edge clears it (≥0.4); a modality edge alone (0.2) is a shrug.
const foreseenAt = 0.3

func liveEmoji(u LiveUpdate) string {
	switch {
	case u.EventType == "home_run":
		return "💣"
	case u.IsScoringPlay:
		return "💥"
	case strings.HasPrefix(u.EventType, "strikeout"):
		return "🔥"
	default:
		return "⚡"
	}
}

// FormatLiveUpdate renders one in-game post. The headline gives way first
// when the post runs long; the cosmic reading always survives.
func FormatLiveUpdate(u LiveUpdate) string {
	half := "Bot"
	if u.IsTop {
		half = "Top"
	}
	head := fmt.Sprintf("%s %s %d · %s %d, %s %d\n", liveEmoji(u), half, u.Inning, u.AwayAbbrev, u.AwayScore, u.HomeAbbrev, u.HomeScore)

	matchup := ""
	if u.SignsKnown {
		matchup = u.Matchup.Describe(Surname(u.Batter), Surname(u.Pitcher)) + "\n"
	}

	var b strings.Builder
	// Stats in plain words: how tense the moment was and how hard the ball
	// was hit. The win-probability swing lives in the 🔮 line's arrow.
	var stats []string
	emoji := "⚾"
	if p := pressureLabel(u.Leverage); p != "" {
		stats, emoji = append(stats, p), "😬"
	}
	switch {
	case u.EventType == "home_run" && u.Distance > 0 && u.LaunchSpeed > 0:
		stats = append(stats, fmt.Sprintf("%.0f ft, %.0f mph off the bat", u.Distance, u.LaunchSpeed))
	case u.LaunchSpeed > 0:
		stats = append(stats, fmt.Sprintf("%.0f mph off the bat", u.LaunchSpeed))
	}
	if len(stats) > 0 {
		fmt.Fprintf(&b, "%s %s\n", emoji, strings.Join(stats, " · "))
	}
	fmt.Fprintf(&b, "🔮 %s win chances: science %.0f%%%s · the stars %.0f%%%s",
		possessive(u.Team), u.Real, cosmicDelta(u.WPA), u.Cosmic, cosmicDelta(u.Cosmic-u.PrevCosmic))
	numbers := b.String()

	// The headline gives way first (down to a stub), then the reaction; the
	// matchup, stats and cosmic reading always make it.
	fixed := len([]rune(head)) + len([]rune("\n\n"+matchup+"\n")) + len([]rune(numbers))
	room := MaxPostLength - fixed
	headline, reactionRoom := u.Headline, room-len([]rune(u.Headline))
	reaction := liveReaction(u, reactionRoom)
	if reactionRoom < len([]rune(reaction)) {
		headRoom := room - len([]rune(reaction))
		if headRoom < minHeadline {
			headRoom = minHeadline
		}
		headline = Fit(headline, headRoom)
		reactionRoom = room - len([]rune(headline))
	}
	if reactionRoom < 1 {
		return Fit(head+headline+"\n\n"+matchup+numbers, MaxPostLength)
	}
	return head + headline + "\n\n" + matchup + Fit(reaction, reactionRoom) + "\n" + numbers
}

// minHeadline is the shortest a play headline gets trimmed to.
const minHeadline = 50

// pressureLabel describes leverage index (1.0 is an average moment) for
// people who don't know what leverage index is.
func pressureLabel(li float64) string {
	switch {
	case li >= 3.5:
		return "Nail-biter"
	case li >= 2.5:
		return "Huge moment"
	case li >= 1.5:
		return "Big moment"
	default:
		return ""
	}
}

// possessive turns "Rays" into "Rays'" and "Red Sox" into "Red Sox's".
func possessive(team string) string {
	if strings.HasSuffix(team, "s") {
		return team + "'"
	}
	return team + "'s"
}

func cosmicDelta(d float64) string {
	n := math.Round(d)
	switch {
	case n >= 1:
		return fmt.Sprintf(" (▲%.0f)", n)
	case n <= -1:
		return fmt.Sprintf(" (▼%.0f)", -n)
	default:
		return ""
	}
}

// Surname turns "Vladimir Guerrero Jr." into "Guerrero".
func Surname(name string) string {
	parts := strings.Fields(name)
	for len(parts) > 1 {
		switch strings.TrimSuffix(parts[len(parts)-1], ".") {
		case "Jr", "Sr", "II", "III", "IV":
			parts = parts[:len(parts)-1]
			continue
		}
		break
	}
	if len(parts) == 0 {
		return name
	}
	return parts[len(parts)-1]
}

// FirstSentence trims MLB's play description ("Aranda singles on a ground
// ball to left fielder Steven Kwan. Ben Williamson scores. ...") to its lead,
// dropping any replay-review preamble ("Bailey challenged (pitch result),
// call on the field was confirmed: DeLuca walks.").
func FirstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ": "); i > 0 && (strings.Contains(s[:i], "challenged") || strings.Contains(s[:i], "review")) {
		s = strings.TrimSpace(s[i+2:])
	}
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}
