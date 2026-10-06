// Package astro is the bot's entire scientific department: zodiac signs from
// birth dates, and how well each sign gets along with the Rockies (a Cancer,
// born July 5, 1991).
package astro

import (
	"fmt"
	"time"
)

type Sign int

const (
	Aries Sign = iota
	Taurus
	Gemini
	Cancer
	Leo
	Virgo
	Libra
	Scorpio
	Sagittarius
	Capricorn
	Aquarius
	Pisces
)

type signInfo struct {
	name    string
	emoji   string
	element string
	// compat is the sign's compatibility with Cancer, 0 (mismatch) to 3 (soulmates).
	compat int
	trait  string
}

var signs = [12]signInfo{
	Aries:       {"Aries", "♈", "fire", 0, "Swings at the first pitch of every at-bat and every life decision."},
	Taurus:      {"Taurus", "♉", "earth", 2, "Stubborn, reliable, and will not be rushed out of the box."},
	Gemini:      {"Gemini", "♊", "air", 1, "Two players in one. Unclear which one shows up at Coors."},
	Cancer:      {"Cancer", "♋", "water", 3, "One of us. Emotionally pre-adjusted for 100 losses."},
	Leo:         {"Leo", "♌", "fire", 1, "Will absolutely bat-flip while down nine runs."},
	Virgo:       {"Virgo", "♍", "earth", 2, "Has already filed a complaint about the infield dirt."},
	Libra:       {"Libra", "♎", "air", 0, "Takes every close pitch. Values balance, especially in the count."},
	Scorpio:     {"Scorpio", "♏", "water", 3, "Intense. Holds a personal grudge against the entire NL West."},
	Sagittarius: {"Sagittarius", "♐", "fire", 0, "Loves to travel. Statistically, toward a contender."},
	Capricorn:   {"Capricorn", "♑", "earth", 1, "Treats spring training like a five-year plan."},
	Aquarius:    {"Aquarius", "♒", "air", 0, "Has opinions about launch angle that nobody asked for."},
	Pisces:      {"Pisces", "♓", "water", 3, "Dreamy, intuitive, and comfortable underwater. Like our playoff odds."},
}

func (s Sign) valid() bool     { return s >= Aries && s <= Pisces }
func (s Sign) String() string  { return s.Name() }
func (s Sign) Name() string    { return signs[s].name }
func (s Sign) Emoji() string   { return signs[s].emoji }
func (s Sign) Element() string { return signs[s].element }
func (s Sign) Trait() string   { return signs[s].trait }
func (s Sign) IsWater() bool   { return signs[s].element == "water" }

// Compatibility with Cancer, 0-3.
func (s Sign) Compatibility() int { return signs[s].compat }

// CompatibilityLabel describes a 0-3 compatibility score in Rockiscope terms.
func CompatibilityLabel(score int) string {
	switch {
	case score >= 3:
		return "soulmates"
	case score == 2:
		return "a solid match"
	case score == 1:
		return "it's complicated"
	default:
		return "a cosmic mismatch"
	}
}

// signStarts lists the first day of each sign, in calendar order from Capricorn's
// January tail. A date belongs to the last entry whose start is <= it.
var signStarts = []struct {
	month time.Month
	day   int
	sign  Sign
}{
	{time.January, 1, Capricorn},
	{time.January, 20, Aquarius},
	{time.February, 19, Pisces},
	{time.March, 21, Aries},
	{time.April, 20, Taurus},
	{time.May, 21, Gemini},
	{time.June, 21, Cancer},
	{time.July, 23, Leo},
	{time.August, 23, Virgo},
	{time.September, 23, Libra},
	{time.October, 23, Scorpio},
	{time.November, 22, Sagittarius},
	{time.December, 22, Capricorn},
}

// SignForDate returns the sun sign for a month/day.
func SignForDate(month time.Month, day int) Sign {
	sign := Capricorn
	for _, s := range signStarts {
		if month > s.month || (month == s.month && day >= s.day) {
			sign = s.sign
		}
	}
	return sign
}

// SignFor parses a "2006-01-02" birth date. ok is false if it can't be parsed.
func SignFor(birthDate string) (Sign, bool) {
	t, err := time.Parse("2006-01-02", birthDate)
	if err != nil {
		return 0, false
	}
	return SignForDate(t.Month(), t.Day()), true
}

// RosterReading is the astrological profile of a whole roster.
type RosterReading struct {
	Players    int     `json:"players"`
	Score      float64 `json:"score"`      // mean Cancer compatibility, 0-3
	WaterShare float64 `json:"waterShare"` // fraction of water signs, 0-1
	Dominant   string  `json:"dominant"`   // most common sign
}

// ReadRoster scores a roster from its players' birth dates. Unparseable dates
// are skipped.
func ReadRoster(birthDates []string) RosterReading {
	var counts [12]int
	var r RosterReading
	total, water := 0, 0
	for _, bd := range birthDates {
		s, ok := SignFor(bd)
		if !ok {
			continue
		}
		counts[s]++
		r.Players++
		total += s.Compatibility()
		if s.IsWater() {
			water++
		}
	}
	if r.Players == 0 {
		return r
	}
	r.Score = float64(total) / float64(r.Players)
	r.WaterShare = float64(water) / float64(r.Players)

	best := Sign(0)
	for s := Aries; s <= Pisces; s++ {
		if counts[s] > counts[best] {
			best = s
		}
	}
	r.Dominant = best.Name()
	return r
}

// Summary is a one-line roster reading, e.g. "🌊 31% water signs · Cancer compatibility 1.8/3".
func (r RosterReading) Summary() string {
	return fmt.Sprintf("🌊 %.0f%% water signs · Cancer compatibility %.1f/3", r.WaterShare*100, r.Score)
}

// Better reports whether r is a more compatible roster than other.
func (r RosterReading) Better(other RosterReading) bool {
	if r.Score != other.Score {
		return r.Score > other.Score
	}
	return r.WaterShare > other.WaterShare
}

// ParseSign looks up a sign by name.
func ParseSign(name string) (Sign, bool) {
	for i, s := range signs {
		if s.name == name {
			return Sign(i), true
		}
	}
	return 0, false
}
