package astro

import (
	"fmt"
	"strings"
)

// Modality is a sign's mode: cardinal signs start things, fixed signs refuse
// to stop them, mutable signs wander off halfway through.
func (s Sign) Modality() string {
	switch s {
	case Aries, Cancer, Libra, Capricorn:
		return "cardinal"
	case Taurus, Leo, Scorpio, Aquarius:
		return "fixed"
	default:
		return "mutable"
	}
}

// elementBeats is the elemental food chain: water douses fire, fire consumes
// air, air erodes earth, earth absorbs water. Fire/earth and water/air are
// indifferent to each other.
var elementBeats = map[string]string{
	"water": "fire",
	"fire":  "air",
	"air":   "earth",
	"earth": "water",
}

var elementVerbs = map[string]string{
	"water": "douses",
	"fire":  "consumes",
	"air":   "erodes",
	"earth": "absorbs",
}

// modalityBeats: initiative beats stubbornness, stubbornness outlasts
// adaptability, adaptability dodges initiative.
var modalityBeats = map[string]string{
	"cardinal": "fixed",
	"fixed":    "mutable",
	"mutable":  "cardinal",
}

const (
	elementEdge  = 0.6
	modalityEdge = 0.2
)

// Matchup is the stars' read on one batter vs one pitcher.
type Matchup struct {
	Batter, Pitcher Sign
	// Edge is the batter's cosmic advantage, -0.8 (pitcher owns him) to 0.8.
	// Elements decide it (±0.6); modality only tips it (±0.2).
	Edge float64
	// Elemental is +1 when the batter's element beats the pitcher's, -1 when
	// the pitcher's wins, 0 when neither does.
	Elemental int
}

// ReadMatchup scores a batter's sign against a pitcher's.
func ReadMatchup(batter, pitcher Sign) Matchup {
	m := Matchup{Batter: batter, Pitcher: pitcher}
	if batter == pitcher {
		return m
	}

	be, pe := batter.Element(), pitcher.Element()
	switch {
	case elementBeats[be] == pe:
		m.Edge += elementEdge
		m.Elemental = 1
	case elementBeats[pe] == be:
		m.Edge -= elementEdge
		m.Elemental = -1
	}

	bm, pm := batter.Modality(), pitcher.Modality()
	switch {
	case modalityBeats[bm] == pm:
		m.Edge += modalityEdge
	case modalityBeats[pm] == bm:
		m.Edge -= modalityEdge
	}
	return m
}

// Describe explains the matchup with each element pinned to its player, e.g.
// "♌ Rasmussen's fire consumes ♊ Bailey's air."
func (m Matchup) Describe(batter, pitcher string) string {
	b := fmt.Sprintf("%s %s %s", m.Batter.Emoji(), possessive(batter), m.Batter.Element())
	p := fmt.Sprintf("%s %s %s", m.Pitcher.Emoji(), possessive(pitcher), m.Pitcher.Element())
	switch {
	case m.Batter == m.Pitcher:
		return fmt.Sprintf("%s %s vs %s %s: a %s mirror match. Nobody blinks.", m.Batter.Emoji(), batter, m.Pitcher.Emoji(), pitcher, m.Batter.Name())
	case m.Elemental > 0:
		return fmt.Sprintf("%s %s %s.", b, elementVerbs[m.Batter.Element()], p)
	case m.Elemental < 0:
		return fmt.Sprintf("%s %s %s.", p, elementVerbs[m.Pitcher.Element()], b)
	case m.Batter.Element() == m.Pitcher.Element():
		return fmt.Sprintf("%s %s vs %s %s: both %s. Someone's getting %s.", m.Batter.Emoji(), batter, m.Pitcher.Emoji(), pitcher, m.Batter.Element(), sameElementFate[m.Batter.Element()])
	default:
		return fmt.Sprintf("%s and %s ignore each other.", b, p)
	}
}

var sameElementFate = map[string]string{
	"fire":  "burned",
	"water": "soaked",
	"earth": "buried",
	"air":   "ghosted",
}

// possessive: "Bailey's", "Hicks'".
func possessive(name string) string {
	if strings.HasSuffix(name, "s") {
		return name + "'"
	}
	return name + "'s"
}
