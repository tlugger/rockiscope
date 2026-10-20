package formatter

import (
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The bot's voice: a dry, slightly unwell astrologer who takes baseball far
// too seriously. Mostly deadpan; capital letters are for emergencies.

// seedOf turns any stable string (a date, a game, a horoscope) into a pick
// seed, so a post reads the same if it's ever rebuilt.
func seedOf(parts ...interface{}) int {
	h := fnv.New32a()
	fmt.Fprint(h, parts...)
	return int(h.Sum32() & 0x7fffffff)
}

func pick(options []string, seed int) string {
	if len(options) == 0 {
		return ""
	}
	return options[abs(seed)%len(options)]
}

// fitting renders a post with every optional extra, then drops extras from
// the end of order (least important last) until it fits.
func fitting(order []string, render func(on map[string]bool) string) string {
	on := make(map[string]bool, len(order))
	for _, k := range order {
		on[k] = true
	}
	text := render(on)
	for i := len(order) - 1; i >= 0 && utf8.RuneCountInString(text) > MaxPostLength; i-- {
		on[order[i]] = false
		text = render(on)
	}
	return Fit(text, MaxPostLength)
}

// ── Horoscope roast ──────────────────────────────────────────────────

const (
	minQuote = 14
	maxQuote = 48
)

// horoscopeQuote pulls a short, quotable clause out of today's horoscope:
// "you might enjoy this day by being lazy". Empty if nothing fits.
func horoscopeQuote(text string, seed int) string {
	var quotes []string
	for _, sentence := range splitSentences(text) {
		for _, clause := range splitClauses(sentence) {
			clause = strings.TrimSpace(strings.TrimRight(clause, ".!?,;: "))
			n := utf8.RuneCountInString(clause)
			if n < minQuote || n > maxQuote || strings.Contains(clause, "Rockies") || strings.Contains(clause, "Cancer") {
				continue
			}
			quotes = append(quotes, lowerFirst(clause))
		}
	}
	return pick(quotes, seed)
}

func splitSentences(text string) []string {
	var out []string
	start := 0
	for i, r := range text {
		if (r == '.' || r == '!' || r == '?') && (i+1 == len(text) || text[i+1] == ' ') {
			out = append(out, text[start:i+1])
			start = i + 1
		}
	}
	if rest := strings.TrimSpace(text[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

// splitClauses breaks a sentence at commas, semicolons and the joints of
// compound sentences, where a quote still reads as a complete thought.
func splitClauses(sentence string) []string {
	parts := []string{sentence}
	for _, sep := range []string{"; ", ", ", " but ", " and ", " so "} {
		var next []string
		for _, p := range parts {
			next = append(next, strings.Split(p, sep)...)
		}
		parts = next
	}
	return parts
}

func lowerFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	// Keep "I" and proper-noun-looking openers ("Mercury") as they are.
	if r == 'I' && (len(s) == size || s[size] == ' ' || s[size] == '\'') {
		return s
	}
	if word := strings.Fields(s)[0]; isSkyWord(word) {
		return s
	}
	return string(unicode.ToLower(r)) + s[size:]
}

func isSkyWord(w string) bool {
	switch strings.Trim(w, ",.") {
	case "Mercury", "Venus", "Mars", "Jupiter", "Saturn", "Uranus", "Neptune", "Pluto", "Moon", "Sun":
		return true
	}
	return false
}

var roastGameDay = []string{
	"The bullpen took that personally.",
	"Noted. Forwarding to the lineup.",
	"The stars wrote that specifically about the seventh inning.",
	"Read that again, but in a dugout.",
	"Unclear if that applies to the bullpen. Assuming yes.",
	"Somebody tell the leadoff hitter.",
	"Saturn would like that framed in the clubhouse.",
}

var roastOffDay = []string{
	"On an off day? Finally, advice we can follow.",
	"The Rockies are taking this extremely literally.",
	"Somewhere a reliever is reading this and nodding.",
	"No game to ruin it. A rare clean reading.",
	"Saving this one for the next homestand.",
}

var roastCountdown = []string{
	"That's the energy we're bringing to Opening Day.",
	"Pinning this in the clubhouse until spring.",
	"The front office has been notified. They did not reply.",
	"Words to live by. Words to rebuild by.",
}

// horoscopeRoast is one line quoting today's horoscope back at us, e.g.
// `📜 Today's horoscope: "you might enjoy this day by being lazy." Noted.`
func horoscopeRoast(text string, seed int, reactions []string) string {
	q := horoscopeQuote(text, seed)
	if q == "" {
		return ""
	}
	return fmt.Sprintf("📜 Today's horoscope: \"%s.\" %s", q, pick(reactions, seed/3))
}

// ── Pregame nuggets ──────────────────────────────────────────────────

var homeVenueQuips = []string{
	"Altitude: spiritually significant.",
	"Thin air, thick hope.",
	"Where fly balls go to find themselves.",
	"Elevation 5,280. Expectations lower.",
}

var awayVenueQuips = []string{
	"Hostile territory.",
	"The stars are traveling economy.",
	"Unfamiliar skies.",
	"Road game. Pack a crystal.",
}

// eraQuip reacts to a starter's ERA. Most starters are just fine and get nothing.
func eraQuip(era float64, seed int) string {
	switch {
	case era >= 6:
		return pick([]string{"Yes, really.", "That's not a typo.", "Thoughts and prayers.", "Bold choice."}, seed)
	case era >= 4.75:
		return pick([]string{"Respectfully, hmm.", "Fine. Probably.", "Vibes: mixed."}, seed)
	case era > 0 && era < 3:
		return pick([]string{"An actual ace. Don't jinx it.", "Trustworthy. For now.", "The stars approve."}, seed)
	}
	return ""
}

// streakQuip reacts to a W/L streak code like "W4" or "L6".
func streakQuip(code string, seed int) string {
	if len(code) < 2 {
		return ""
	}
	var n int
	fmt.Sscanf(code[1:], "%d", &n)
	switch {
	case code[0] == 'W' && n >= 5:
		return pick([]string{"Who ARE we?", "Nobody breathe.", "Unprecedented. Literally."}, seed)
	case code[0] == 'W' && n >= 3:
		return pick([]string{"Suspicious.", "Hot. Don't touch it.", "The stars are nervous."}, seed)
	case code[0] == 'L' && n >= 6:
		return pick([]string{"Send help.", "I've stopped counting. The stars haven't.", "Rock bottom has a basement."}, seed)
	case code[0] == 'L' && n >= 3:
		return pick([]string{"Concerning.", "Thoughts and prayers.", "Mercury is involved."}, seed)
	}
	return ""
}

// runDiffQuip reacts to a season run differential worth reacting to.
func runDiffQuip(diff int, seed int) string {
	switch {
	case diff <= -75:
		return pick([]string{"That's not a typo.", "Scientists are studying it."}, seed)
	case diff <= -25:
		return pick([]string{"Yikes.", "We don't talk about it."}, seed)
	case diff >= 25:
		return pick([]string{"Who ARE we?", "Mathematically, good."}, seed)
	}
	return ""
}

// predictionKicker follows the 🔮 line, sized to how sure the stars are.
func predictionKicker(prob float64, seed int) string {
	d := prob - 0.5
	if d < 0 {
		d = -d
	}
	switch {
	case d > 0.30:
		return pick([]string{"Saturn is YELLING.", "The stars have never been more sure of anything.", "Please make the stars stop."}, seed)
	case d > 0.20:
		return pick([]string{"Mercury signed off on it.", "The cosmos are fairly sure about this one.", "Strongly worded, cosmically."}, seed)
	case d > 0.10:
		return pick([]string{"The stars have a feeling.", "Trust the process. The process is astrology.", "Moderate cosmic confidence."}, seed)
	case d > 0.05:
		return pick([]string{"I don't make the rules. Saturn does.", "Barely. Don't read into it.", "A whisper from the void."}, seed)
	default:
		return pick([]string{"Flip a coin. We did.", "Saturn abstained.", "The universe shrugged."}, seed)
	}
}

func withQuip(line, quip string) string {
	if quip == "" {
		return line
	}
	return line + " " + quip
}
