package formatter

import (
	"strings"
	"testing"

	"github.com/tlugger/rockiscope/internal/astro"
)

func sampleLiveUpdate() LiveUpdate {
	return LiveUpdate{
		Team: "Rays", Inning: 2, IsTop: true,
		AwayAbbrev: "TB", AwayScore: 2, HomeAbbrev: "CLE", HomeScore: 0,
		EventType: "single", IsScoringPlay: true,
		Headline: "Jonathan Aranda's RBI single",
		Batter:   "Jonathan Aranda", Pitcher: "Parker Messick",
		SignsKnown: true,
		Matchup:    astro.ReadMatchup(astro.Gemini, astro.Taurus),
		Destiny:    0.4, HelpedUs: true, BatterWon: true,
		WPA: 8.8, Leverage: 1.79, LaunchSpeed: 85.5,
		Real: 72.8, Cosmic: 77.9, PrevCosmic: 70.1,
	}
}

func TestFormatLiveUpdate(t *testing.T) {
	text := FormatLiveUpdate(sampleLiveUpdate())
	for _, want := range []string{
		"💥 Top 2 · TB 2, CLE 0\nJonathan Aranda's RBI single\n\n",
		"♊ Aranda's air erodes ♉ Messick's earth.\n",
		"\n😬 Big moment · 86 mph off the bat\n",
		"🔮 Rays' win chances: science 73% (▲9) · the stars 78% (▲8)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// The stars called it (an elemental edge for the batter who came through).
	if !containsAny(text, destinyTags["foreseen+good"]) {
		t.Errorf("missing a foreseen tag:\n%s", text)
	}
	if n := len([]rune(text)); n > MaxPostLength {
		t.Errorf("%d runes, over the limit:\n%s", n, text)
	}
}

func containsAny(text string, options []string) bool {
	for _, o := range options {
		if strings.Contains(text, o) {
			return true
		}
	}
	return false
}

func TestLiveReaction_DestinyTags(t *testing.T) {
	cases := []struct {
		destiny float64
		helped  bool
		pool    string
	}{
		{0.6, true, "foreseen+good"},
		{0.6, false, "foreseen+bad"},
		{-0.8, true, "fluke+good"},
		{-0.6, false, "fluke+bad"},
	}
	for _, c := range cases {
		u := sampleLiveUpdate()
		u.Destiny, u.HelpedUs = c.destiny, c.helped
		if r := liveReaction(u, 300); !containsAny(r, destinyTags[c.pool]) {
			t.Errorf("destiny %.1f helped %v: no %s tag in %q", c.destiny, c.helped, c.pool, r)
		}
	}
	// A modality edge alone isn't fate: no tag.
	u := sampleLiveUpdate()
	u.Destiny = 0.2
	r := liveReaction(u, 300)
	for _, tags := range destinyTags {
		if containsAny(r, tags) {
			t.Errorf("shrug got a destiny tag: %q", r)
		}
	}
}

// Every template, every sign pairing, both directions: placeholders all
// filled, grammar intact, and no gendered pronouns for players.
func TestLiveReaction_AllTemplatesRender(t *testing.T) {
	for _, event := range []string{"home_run", "strikeout", "grounded_into_double_play", "walk", "single", "sac_fly", "field_out"} {
		for b := astro.Aries; b <= astro.Pisces; b++ {
			for p := astro.Aries; p <= astro.Pisces; p++ {
				for _, batterWon := range []bool{true, false} {
					u := sampleLiveUpdate()
					u.EventType, u.BatterWon, u.Matchup, u.Destiny = event, batterWon, astro.ReadMatchup(b, p), 0
					for pick := 0; pick < 6; pick++ {
						r := reactionAt(u, pick)
						if strings.ContainsAny(r, "{}") {
							t.Fatalf("unfilled placeholder: %q", r)
						}
						for _, bad := range []string{"Ariess", "Piscess", "ARIESS", "PISCESS", "a Aries", "a Aquarius", " he ", " his ", " she ", " her "} {
							if strings.Contains(" "+r+" ", bad) {
								t.Fatalf("%q in %q", bad, r)
							}
						}
					}
				}
			}
		}
	}
}

func TestPlayKind(t *testing.T) {
	for _, c := range []struct {
		event     string
		batterWon bool
		want      string
	}{
		{"home_run", true, "homer"},
		{"strikeout_double_play", false, "strikeout"},
		{"grounded_into_double_play", false, "doubleplay"},
		{"hit_by_pitch", true, "walk"},
		{"double", true, "hit"},
		{"sac_fly", true, "batter"},
		{"field_out", false, "pitcher"},
	} {
		if got := playKind(c.event, c.batterWon); got != c.want {
			t.Errorf("playKind(%s, %v) = %s, want %s", c.event, c.batterWon, got, c.want)
		}
	}
}

func TestLiveReaction_HeroIsWhoeverWon(t *testing.T) {
	u := sampleLiveUpdate()
	u.EventType, u.BatterWon = "strikeout", false
	u.Matchup = astro.ReadMatchup(astro.Gemini, astro.Scorpio)
	u.Destiny = 0
	for pick := 0; pick < len(reactionTemplates["strikeout"]); pick++ {
		r := reactionAt(u, pick)
		if strings.Contains(r, "Aranda struck out") || strings.Contains(r, "SIT DOWN, SCORPIO") {
			t.Errorf("roles swapped: %q", r)
		}
	}
}

func TestLiveReaction_UnknownSigns(t *testing.T) {
	u := sampleLiveUpdate()
	u.SignsKnown = false
	r := liveReaction(u, 300)
	if !strings.Contains(r, "Aranda") || !containsAny(r, []string{"birth", "read"}) {
		t.Errorf("signless reaction = %q", r)
	}
	text := FormatLiveUpdate(u)
	if strings.Contains(text, "♊") {
		t.Errorf("no matchup line without signs:\n%s", text)
	}
}

func TestLiveReaction_PicksOneThatFits(t *testing.T) {
	u := sampleLiveUpdate()
	shortest := 1000
	for pick := 0; pick < 4; pick++ {
		if n := len([]rune(reactionAt(u, pick))); n < shortest {
			shortest = n
		}
	}
	if r := liveReaction(u, shortest); len([]rune(r)) > shortest {
		t.Errorf("%q is over %d runes", r, shortest)
	}
}

func TestFormatLiveUpdate_LongHeadlineFits(t *testing.T) {
	u := sampleLiveUpdate()
	u.Headline = strings.Repeat("Patrick Bailey grounds into a double play, second baseman Ben Williamson to shortstop Taylor Walls. ", 3)
	text := FormatLiveUpdate(u)
	if n := len([]rune(text)); n > MaxPostLength {
		t.Errorf("%d runes:\n%s", n, text)
	}
	if !strings.HasSuffix(text, "the stars 78% (▲8)") || !strings.Contains(text, "…") || !strings.Contains(text, "♊ Aranda's air") {
		t.Errorf("the headline should be trimmed, not the reading:\n%s", text)
	}
}

func TestFormatLiveUpdate_HomeRun(t *testing.T) {
	u := sampleLiveUpdate()
	u.EventType, u.Distance, u.IsTop = "home_run", 421, false
	u.PrevCosmic = u.Cosmic
	text := FormatLiveUpdate(u)
	if !strings.HasPrefix(text, "💣 Bot 2") || !strings.Contains(text, "421 ft, 86 mph off the bat") {
		t.Errorf("unexpected:\n%s", text)
	}
	if !strings.HasSuffix(text, "the stars 78%") {
		t.Errorf("no change in the stars should show no arrow:\n%s", text)
	}
}

func TestFormatLiveUpdate_PlainWords(t *testing.T) {
	u := sampleLiveUpdate()
	u.Leverage, u.LaunchSpeed, u.WPA, u.Team = 0.9, 0, -12.4, "Red Sox"
	text := FormatLiveUpdate(u)
	if strings.Contains(text, "😬") || strings.Contains(text, "⚾") {
		t.Errorf("a routine moment with no batted ball needs no stat line:\n%s", text)
	}
	if !strings.Contains(text, "🔮 Red Sox's win chances: science 73% (▼12)") {
		t.Errorf("unexpected:\n%s", text)
	}
	u.LaunchSpeed = 70
	if text := FormatLiveUpdate(u); !strings.Contains(text, "\n⚾ 70 mph off the bat\n") {
		t.Errorf("a calm moment shouldn't wince:\n%s", text)
	}
	for li, want := range map[float64]string{1.2: "", 1.79: "Big moment", 2.8: "Huge moment", 3.7: "Nail-biter"} {
		if got := pressureLabel(li); got != want {
			t.Errorf("pressureLabel(%.2f) = %q, want %q", li, got, want)
		}
	}
}

func TestSurname(t *testing.T) {
	for in, want := range map[string]string{
		"Jonathan Aranda":       "Aranda",
		"Vladimir Guerrero Jr.": "Guerrero",
		"Ken Griffey Sr":        "Griffey",
		"Ichiro":                "Ichiro",
	} {
		if got := Surname(in); got != want {
			t.Errorf("Surname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstSentence(t *testing.T) {
	for in, want := range map[string]string{
		"Jonathan Aranda singles on a ground ball to left fielder Steven Kwan.   Ben Williamson scores.":                      "Jonathan Aranda singles on a ground ball to left fielder Steven Kwan.",
		"Patrick Bailey challenged (pitch result), call on the field was confirmed: Jonny DeLuca walks.   Yandy Díaz scores.": "Jonny DeLuca walks.",
	} {
		if got := FirstSentence(in); got != want {
			t.Errorf("FirstSentence(%q) = %q, want %q", in, got, want)
		}
	}
}
