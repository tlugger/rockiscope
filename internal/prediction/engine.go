package prediction

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/tlugger/rockiscope/internal/mlb"
)

// Prediction is the output of the prediction engine.
type Prediction struct {
	WinProbability float64
	Pick           string // "W" or "L"
	Confidence     string // human-readable confidence descriptor
	Factors        map[string]float64
}

// Input holds all the data the engine needs. Any field can be nil/zero
// if data is unavailable — the engine gracefully degrades.
type Input struct {
	Record          *mlb.TeamRecord
	RockiesPitcher  *mlb.PitcherStats
	OpponentPitcher *mlb.PitcherStats
	HeadToHead      *mlb.H2HRecord
	IsHome          bool
	HoroscopeText   string
}

// Factor weights — must sum to 1.0.
const (
	weightWinRate  = 0.30
	weightPitcher  = 0.30
	weightH2H      = 0.15
	weightHomeAway = 0.10
	weightMomentum = 0.05
	weightStars    = 0.10
)

// Predict computes a win probability and pick from the input data.
// All logic is pure — no I/O, fully deterministic for the same inputs.
func Predict(in Input) Prediction {
	factors := make(map[string]float64)
	totalWeight := 0.0
	weightedSum := 0.0

	// 1. Win rate factor
	if in.Record != nil && (in.Record.Wins+in.Record.Losses) > 0 {
		totalWeight += weightWinRate
		weightedSum += weightWinRate * in.Record.WinningPercentage
		factors["winRate"] = in.Record.WinningPercentage
	}

	// 2. Pitcher matchup factor
	pitcherScore := pitcherFactor(in.RockiesPitcher, in.OpponentPitcher)
	if pitcherScore >= 0 {
		totalWeight += weightPitcher
		weightedSum += weightPitcher * pitcherScore
		factors["pitcher"] = pitcherScore
	}

	// 3. Head-to-head factor
	if in.HeadToHead != nil && in.HeadToHead.GamesPlayed > 0 {
		totalWeight += weightH2H
		h2hPct := float64(in.HeadToHead.Wins) / float64(in.HeadToHead.GamesPlayed)
		weightedSum += weightH2H * h2hPct
		factors["h2h"] = h2hPct
	}

	// 4. Home/away factor
	if in.Record != nil {
		totalWeight += weightHomeAway
		homeAwayScore := 0.54
		if in.IsHome {
			homeGames := in.Record.HomeWins + in.Record.HomeLosses
			if homeGames > 0 {
				homeAwayScore = float64(in.Record.HomeWins) / float64(homeGames)
			}
		} else {
			awayGames := in.Record.AwayWins + in.Record.AwayLosses
			if awayGames > 0 {
				homeAwayScore = float64(in.Record.AwayWins) / float64(awayGames)
			} else {
				homeAwayScore = 0.46
			}
		}
		weightedSum += weightHomeAway * homeAwayScore
		factors["homeAway"] = homeAwayScore
	}

	// 5. Momentum factor (streak)
	if in.Record != nil && in.Record.StreakCode != "" {
		totalWeight += weightMomentum
		momentumScore := streakScore(in.Record.StreakCode)
		weightedSum += weightMomentum * momentumScore
		factors["momentum"] = momentumScore
	}

	// 6. Horoscope factor — the stars speak
	if in.HoroscopeText != "" {
		totalWeight += weightStars
		starsScore := horoscopeScore(in.HoroscopeText)
		weightedSum += weightStars * starsScore
		factors["stars"] = starsScore
	}

	// Normalize
	var prob float64
	if totalWeight > 0 {
		prob = weightedSum / totalWeight
	} else {
		prob = horoscopeScore(in.HoroscopeText)
		if prob == 0 {
			prob = 0.5
		}
	}

	// Clamp to [0.05, 0.95] — nothing is certain
	prob = math.Max(0.05, math.Min(0.95, prob))

	pick := "L"
	if prob >= 0.5 {
		pick = "W"
	}

	return Prediction{
		WinProbability: prob,
		Pick:           pick,
		Confidence:     confidenceLabel(prob),
		Factors:        factors,
	}
}

// pitcherFactor compares two pitchers. Returns 0-1 where 1 = Rockies pitcher much better.
// Returns -1 if insufficient data.
func pitcherFactor(rockies, opponent *mlb.PitcherStats) float64 {
	if rockies == nil || opponent == nil {
		return -1
	}
	if rockies.InningsPitched < 3 || opponent.InningsPitched < 3 {
		return -1 // not enough data
	}

	// Compare ERA — lower is better for us when opponent has high ERA
	// Normalize ERA to 0-1 scale: ERA of 0 = 1.0, ERA of 9+ = 0.0
	rockiesERA := math.Max(0, math.Min(9, rockies.ERA))
	opponentERA := math.Max(0, math.Min(9, opponent.ERA))

	rockiesScore := 1 - (rockiesERA / 9.0)
	opponentScore := 1 - (opponentERA / 9.0)

	// Also factor in WHIP (lower is better)
	rockiesWHIP := math.Max(0, math.Min(3, rockies.WHIP))
	opponentWHIP := math.Max(0, math.Min(3, opponent.WHIP))

	rockiesWHIPScore := 1 - (rockiesWHIP / 3.0)
	opponentWHIPScore := 1 - (opponentWHIP / 3.0)

	// Combine: how much better is our pitcher vs theirs
	// Higher rockiesScore = better pitcher for us. Higher opponentScore = worse matchup for us.
	// We want: Rockies good + opponent bad = high result.
	eraAdvantage := (rockiesScore - opponentScore + 1) / 2 // 0-1, 0.5 = even
	whipAdvantage := (rockiesWHIPScore - opponentWHIPScore + 1) / 2

	return eraAdvantage*0.6 + whipAdvantage*0.4
}

// streakScore converts a streak code like "W3" or "L2" into a 0-1 score.
func streakScore(code string) float64 {
	if len(code) < 2 {
		return 0.5
	}
	direction := code[0]
	length := 0
	for _, c := range code[1:] {
		if c >= '0' && c <= '9' {
			length = length*10 + int(c-'0')
		}
	}
	// Cap at 10 games
	if length > 10 {
		length = 10
	}
	score := float64(length) / 10.0 * 0.5 // max 0.5 swing
	if direction == 'W' {
		return 0.5 + score
	}
	return 0.5 - score
}

// horoscopeScore deterministically hashes horoscope text to a 0-1 value.
// The stars are capricious but consistent.
func horoscopeScore(text string) float64 {
	if text == "" {
		return 0.5
	}
	h := sha256.Sum256([]byte(text))
	n := binary.BigEndian.Uint32(h[:4])
	return float64(n) / float64(math.MaxUint32)
}

func confidenceLabel(prob float64) string {
	dist := math.Abs(prob - 0.5)
	switch {
	case dist > 0.30:
		return "The stars are screaming"
	case dist > 0.20:
		return "The cosmos strongly favor"
	case dist > 0.10:
		return "The stars lean toward"
	case dist > 0.05:
		return "A slight celestial nudge toward"
	default:
		return "A cosmic coin flip"
	}
}

// FormatPrediction returns a human-readable prediction string.
func (p Prediction) FormatPrediction() string {
	pct := p.WinProbability * 100
	if p.Pick == "W" {
		return fmt.Sprintf("%s a Rockies victory (%.0f%%)", p.Confidence, pct)
	}
	return fmt.Sprintf("%s a Rockies defeat (%.0f%%)", p.Confidence, 100-pct)
}
