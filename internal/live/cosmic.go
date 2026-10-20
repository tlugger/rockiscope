// Package live follows a game in progress. It watches for notable plays,
// waits for MLB to cut a highlight clip, reads the batter's and pitcher's
// birth charts, and replies to the pregame post with a cosmically corrected
// win probability. The thread ends with the scheduler's usual result post.
package live

import (
	"math"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/mlb"
)

const (
	// maxOffset bounds how far the stars may stray from science, in points.
	maxOffset = 12.0
	// offsetDecay pulls the stars back toward science with each notable play,
	// so an early cosmic grudge fades by the late innings.
	offsetDecay = 0.85
	// destinyGain scales how much of a play's swing the stars amplify (when
	// they saw it coming) or claw back (when they call it a fluke).
	destinyGain = 0.4
	// leverageGain adds weight per unit of leverage index, capped at maxLeverage.
	leverageGain = 0.2
	maxLeverage  = 3.0

	// A play is notable if it scores, leaves the yard, swings win probability
	// by notableWPA points, or happens at notableLeverage or higher.
	notableWPA      = 10.0
	notableLeverage = 2.0
)

// isNotable reports whether a play deserves a post.
func isNotable(p mlb.LivePlay) bool {
	return p.IsScoringPlay || p.EventType == "home_run" || isBigSwing(p)
}

func isBigSwing(p mlb.LivePlay) bool {
	return math.Abs(p.HomeWPA) >= notableWPA || p.Leverage >= notableLeverage
}

// teamWP is the followed team's win probability after the play, 0-100.
func teamWP(p mlb.LivePlay, isHome bool) float64 {
	if isHome {
		return p.HomeWP
	}
	return 100 - p.HomeWP
}

// teamWPA is how much the play moved the followed team's win probability.
func teamWPA(p mlb.LivePlay, isHome bool) float64 {
	if isHome {
		return p.HomeWPA
	}
	return -p.HomeWPA
}

// Reading is the stars' verdict on one play.
type Reading struct {
	Real       float64 // MLB's win probability for our team, 0-100
	Cosmic     float64 // after the stars' correction
	PrevCosmic float64
	WPA        float64 // our team's win probability added
	Matchup    astro.Matchup
	SignsKnown bool
	// Destiny is how strongly the stars predicted this outcome: positive when
	// the batter's cosmic edge matched what happened, negative for a fluke.
	Destiny float64
	// BatterWon is whether the batter's side gained win probability.
	BatterWon bool
	Offset    float64 // the stars' running correction after this play
}

// evaluate reads one notable play. prevOffset is the stars' running
// correction before it.
func evaluate(p mlb.LivePlay, isHome bool, players map[int]mlb.Person, prevOffset float64) Reading {
	r := Reading{
		Real: teamWP(p, isHome),
		WPA:  teamWPA(p, isHome),
	}
	r.PrevCosmic = clampWP(r.Real - r.WPA + prevOffset)

	// How the batter's side did on this play, by win probability.
	batterGain := r.WPA
	if batterIsUs := p.IsTop != isHome; !batterIsUs {
		batterGain = -batterGain
	}
	r.BatterWon = batterGain > 0 || (batterGain == 0 && p.IsScoringPlay)

	batter, bok := astro.SignFor(players[p.BatterID].BirthDate)
	pitcher, pok := astro.SignFor(players[p.PitcherID].BirthDate)
	if bok && pok {
		r.SignsKnown = true
		r.Matchup = astro.ReadMatchup(batter, pitcher)
		r.Destiny = r.Matchup.Edge * sign(batterGain)
	}

	// Foreseen plays get amplified, flukes get partially reversed, and the
	// stakes (leverage) decide how loudly the stars weigh in.
	weight := destinyGain + leverageGain*math.Min(p.Leverage, maxLeverage)
	nudge := r.WPA * r.Destiny * weight
	r.Offset = clamp(prevOffset*offsetDecay+nudge, -maxOffset, maxOffset)
	r.Cosmic = clampWP(r.Real + r.Offset)
	return r
}

// initialOffset seeds the stars' correction from the pregame prediction, so
// the in-game line starts where Rockiscope's own call did.
func initialOffset(pregameWP float64) float64 {
	if pregameWP <= 0 {
		return 0
	}
	return clamp(pregameWP*100-50, -maxOffset, maxOffset)
}

// clampWP keeps the stars humble: never 0%, never 100%.
func clampWP(wp float64) float64 { return clamp(wp, 1, 99) }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
