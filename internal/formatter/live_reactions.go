package formatter

import (
	"strings"

	"github.com/tlugger/rockiscope/internal/astro"
)

// A live reaction is one unhinged line about the play, built from what
// happened (a homer, a strikeout, a double play...) and the sign of whoever
// came out ahead (the hero) or got burned (the goat). Templates name players
// by surname or sign, never by pronoun.
//
// Placeholders: {H}/{G} hero/goat surname, {HS}/{GS} their sign ("Leo"),
// {HSU}/{GSU} the sign in caps, {HSP}/{GSP} plural ("Leos"), {HSPU}/{GSPU}
// plural caps, {HSA}/{GSA} with an article ("an Aries"), {HQ}/{GQ} the sign's
// quirk for that role.

// signQuirks are what each sign brings to a big moment, good and bad. Each
// slots into "...pure {HS} behavior: {HQ}." or "...{GQ} will do that."
var signQuirks = [12]struct{ hero, goat string }{
	astro.Aries:       {"zero impulse control, weaponized", "zero impulse control"},
	astro.Taurus:      {"the stubbornness of a load-bearing wall", "the urgency of a Sunday nap"},
	astro.Gemini:      {"two personalities and BOTH can hit", "two personalities, neither one present"},
	astro.Cancer:      {"the full emotional backing of Rockiscope", "big feelings and no follow-through"},
	astro.Leo:         {"main character energy", "main character energy and no plot"},
	astro.Virgo:       {"spreadsheet-level precision", "a crippling overthinking problem"},
	astro.Libra:       {"perfect cosmic balance", "a total inability to commit"},
	astro.Scorpio:     {"a grudge against the entire dugout", "a grudge and nothing else"},
	astro.Sagittarius: {"reckless optimism", "one foot out the door"},
	astro.Capricorn:   {"a five-year plan, executed", "a five-year plan and no Plan B"},
	astro.Aquarius:    {"galaxy-brain launch angle theories", "opinions nobody asked for"},
	astro.Pisces:      {"dreamy psychic intuition", "dreamy vibes and no idea what the count is"},
}

// playKind buckets MLB's event types into what the reactions riff on.
func playKind(eventType string, batterWon bool) string {
	switch eventType {
	case "home_run":
		return "homer"
	case "strikeout", "strikeout_double_play":
		return "strikeout"
	case "grounded_into_double_play", "double_play", "triple_play", "sac_fly_double_play":
		return "doubleplay"
	case "walk", "intent_walk", "hit_by_pitch":
		return "walk"
	case "single", "double", "triple":
		return "hit"
	}
	if batterWon {
		return "batter"
	}
	return "pitcher"
}

var reactionTemplates = map[string][]string{
	"homer": {
		"{HSU} HOMER. {H} has {HQ} and now the whole ballpark knows it.",
		"{H} hit that ball into {HS} season. NASA has been notified.",
		"{GS} pitchers should NOT throw strikes to {HSP}. I have been saying this for YEARS.",
		"Mercury isn't even in retrograde. {H} did that on pure {HS} spite.",
		"{GS} {G} threw that pitch with {GQ}. GOODBYE, BASEBALL.",
	},
	"strikeout": {
		"{HS} {H} struck out {G} and will remember it FOREVER. {HSP} don't forget.",
		"{G} swung with {GQ}. Which is to say: at NOTHING.",
		"SIT DOWN, {GSU}. The stars told you to sit down and you did not listen.",
		"{H} just struck out {GSA}. Pure {HS} behavior: {HQ}.",
		"{GSPU} AND BREAKING BALLS: A TRAGEDY IN THREE STRIKES.",
	},
	"doubleplay": {
		"{GSA} hitting into TWO OUTS AT ONCE. Honestly? On brand.",
		"{G} brought {GQ} to the plate and left with two outs. Classic {GS}.",
		"TWO OUTS. ONE SWING. {HSPU} CANNOT BE STOPPED.",
		"{GSPU} CANNOT BE TRUSTED WITH RUNNERS ON. I have charts.",
		"{H} turned {GSA} into two outs. The moon is SCREAMING.",
	},
	"walk": {
		"{GS} {G} couldn't find the zone. {GQ} will do that.",
		"{H} stood there like {HSA} and let destiny walk right in.",
		"FOUR BALLS. {GSPU} DO NOT BELIEVE IN THE STRIKE ZONE.",
		"{G} pitched that at-bat with {GQ}. The stars are horrified.",
		"{H} took four balls like {HSA} with {HQ}. Patience is a planet now.",
	},
	"hit": {
		"{H} with the hit. Pure {HS} behavior: {HQ}.",
		"{GS} {G} served that up. NEVER trust {GSA} with a lead.",
		"{HSPU} ARE BUILT DIFFERENT. I have said this. I will keep saying this.",
		"{HS} {H} brought {HQ}. {GS} {G} brought nothing.",
		"{G} grooved one to {HSA}. In THIS moon phase? Unforgivable.",
	},
	"batter": {
		"{H} got it done. Pure {HS} behavior: {HQ}.",
		"{GS} {G} just got out-{HS}'d. I didn't know that was possible either.",
		"{HSU} RISING. {GSU} IN SHAMBLES.",
		"{H} came through with {HQ}. {GS} {G} will need a minute.",
	},
	"pitcher": {
		"{H} got the out. {HSP} simply do not miss under a moon like this.",
		"{G} made an out with {GQ}. EXTREMELY {GSU}.",
		"{HSU} PITCHERS ARE A CHEAT CODE. I will not be taking questions.",
		"{H} handled {GSA} with {HQ}. Next.",
		"{GS} {G} brought {GQ}. And it SHOWED.",
	},
}

// destinyTags open the reaction when the stars had an opinion.
var destinyTags = map[string][]string{
	"foreseen+good": {"AS FORETOLD.", "CALLED IT.", "WRITTEN IN THE STARS."},
	"foreseen+bad":  {"THE STARS WARNED YOU.", "I TRIED TO TELL YOU.", "FATE IS CRUEL."},
	"fluke+good":    {"A FLUKE. WE'LL TAKE IT.", "THE CHARTS SAID NO.", "ASTROLOGICALLY ILLEGAL."},
	"fluke+bad":     {"RIGGED.", "SOMEONE ELSE IS DOING THIS.", "THIS WAS NOT IN THE CHARTS."},
}

// signlessReactions are for players MLB has no birthday on, which is its own
// kind of suspicious.
var signlessReactions = map[bool][]string{
	true: {
		"No birthday on file for {H}. Possibly not of this Earth. Thrilled either way.",
		"The stars can't read {H}. NOBODY can read {H}.",
	},
	false: {
		"{H} has no birthday on file. That explains EVERYTHING.",
		"MLB won't release {H}'s birth date. What are they hiding?",
	},
}

// liveReaction picks a reaction no longer than max runes when the pool has
// one (starting from the play's seed, so it's stable across restarts), else
// the shortest.
func liveReaction(u LiveUpdate, max int) string {
	best := ""
	for i := 0; i < 4; i++ {
		line := reactionAt(u, abs(u.Seed)+i)
		if len([]rune(line)) <= max {
			return line
		}
		if best == "" || len([]rune(line)) < len([]rune(best)) {
			best = line
		}
	}
	return best
}

func reactionAt(u LiveUpdate, pick int) string {
	hero, goat := Surname(u.Batter), Surname(u.Pitcher)
	heroSign, goatSign := u.Matchup.Batter, u.Matchup.Pitcher
	if !u.BatterWon {
		hero, goat = goat, hero
		heroSign, goatSign = goatSign, heroSign
	}
	if !u.SignsKnown {
		pool := signlessReactions[u.HelpedUs]
		return strings.ReplaceAll(pool[pick%len(pool)], "{H}", hero)
	}

	pool := reactionTemplates[playKind(u.EventType, u.BatterWon)]
	hp, gp := signPlural(heroSign), signPlural(goatSign)
	line := strings.NewReplacer(
		"{H}", hero, "{G}", goat,
		"{HSPU}", strings.ToUpper(hp), "{GSPU}", strings.ToUpper(gp),
		"{HSP}", hp, "{GSP}", gp,
		"{HSU}", strings.ToUpper(heroSign.Name()), "{GSU}", strings.ToUpper(goatSign.Name()),
		"{HSA}", withArticle(heroSign), "{GSA}", withArticle(goatSign),
		"{HS}", heroSign.Name(), "{GS}", goatSign.Name(),
		"{HQ}", signQuirks[heroSign].hero, "{GQ}", signQuirks[goatSign].goat,
	).Replace(pool[pick%len(pool)])
	line = strings.ToUpper(line[:1]) + line[1:]

	if tags := destinyTags[destinyKind(u)]; len(tags) > 0 {
		line = tags[pick%len(tags)] + " " + line
	}
	return line
}

func destinyKind(u LiveUpdate) string {
	kind := ""
	switch {
	case u.Destiny >= foreseenAt:
		kind = "foreseen"
	case u.Destiny <= -foreseenAt:
		kind = "fluke"
	default:
		return ""
	}
	if u.HelpedUs {
		return kind + "+good"
	}
	return kind + "+bad"
}

// signPlural: "Leos", but Aries and Pisces are their own plural.
func signPlural(s astro.Sign) string {
	if strings.HasSuffix(s.Name(), "s") {
		return s.Name()
	}
	return s.Name() + "s"
}

// withArticle: "a Leo", "an Aries".
func withArticle(s astro.Sign) string {
	if strings.ContainsRune("AEIOU", rune(s.Name()[0])) {
		return "an " + s.Name()
	}
	return "a " + s.Name()
}
