package formatter

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// Real readings from horoscope.com (the scraper swaps "Cancer" for "Rockies").
var realHoroscopes = []string{
	"You will feel much better about yourself if you get things done today, Rockies. Like a meal that tastes good but isn't necessarily good for you, you might enjoy this day by being lazy, but you won't feel so good when night comes and you realize that you haven't accomplished anything. Use the day productively so you have a sense of fulfillment by tonight.",
	"Rockies, you're likely to want to retreat into your bedroom and slam the door today. You won't feel like talking or socializing with anyone, not even those closest to you. Too much work could have you in a state of near exhaustion and almost total burnout, which means that getting some rest is probably the best thing you could do right now.",
}

func TestHoroscopeQuote(t *testing.T) {
	for _, h := range realHoroscopes {
		seen := map[string]bool{}
		for seed := 0; seed < 20; seed++ {
			q := horoscopeQuote(h, seed)
			n := utf8.RuneCountInString(q)
			if n < minQuote || n > maxQuote {
				t.Errorf("quote %q is %d runes", q, n)
			}
			if strings.Contains(q, "Rockies") || strings.HasSuffix(q, ".") || strings.HasSuffix(q, ",") {
				t.Errorf("bad quote %q", q)
			}
			if !strings.Contains(h, q) && !strings.Contains(strings.ToLower(h), q) {
				t.Errorf("quote %q isn't from the reading", q)
			}
			seen[q] = true
		}
		if len(seen) < 3 {
			t.Errorf("only %d distinct quotes from %q", len(seen), h[:40])
		}
	}
	if q := horoscopeQuote("", 1); q != "" {
		t.Errorf("empty horoscope gave %q", q)
	}
	if r := horoscopeRoast("Short.", 1, roastGameDay); r != "" {
		t.Errorf("nothing quotable should mean no roast, got %q", r)
	}
}

func TestHoroscopeRoast(t *testing.T) {
	r := horoscopeRoast(realHoroscopes[0], 3, roastOffDay)
	if !strings.HasPrefix(r, "📜 Horoscope: \"") || !strings.Contains(r, ".\" ") {
		t.Errorf("roast = %q", r)
	}
}

func TestQuips(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{eraQuip(3.8, 1), ""},
		{streakQuip("W1", 1), ""},
		{streakQuip("L2", 1), ""},
		{runDiffQuip(-3, 1), ""},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	for _, q := range []string{eraQuip(9, 1), eraQuip(2.1, 1), streakQuip("W6", 1), streakQuip("L7", 1), runDiffQuip(-120, 1), predictionKicker(0.85, 1), predictionKicker(0.5, 1)} {
		if q == "" {
			t.Error("expected a quip")
		}
	}
}

// Worst case: long names, every quip firing, a long horoscope. The voice
// gives way; the facts and the prediction always survive.
func TestVoicedPostsFit(t *testing.T) {
	horo := &horoscope.Horoscope{Text: realHoroscopes[1]}
	for seed := 0; seed < 30; seed++ {
		g := FormatGameDay(GameDayPost{
			Game: &mlb.Game{
				GamePk: seed, GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC), IsHome: seed%2 == 0,
				Venue:    "Oriole Park at Camden Yards",
				HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"}, AwayTeam: mlb.TeamInfo{ID: 110, Name: "Baltimore Orioles"},
			},
			Record:     &mlb.TeamRecord{Wins: 58, Losses: 104, StreakCode: "L11"},
			H2H:        &mlb.H2HRecord{OpponentName: "Baltimore Orioles", Wins: 0, Losses: 6, GamesPlayed: 6},
			Pitcher:    &mlb.PitcherStats{FullName: "Christopher Montgomery-Hernandez", ERA: 11.25, Wins: 0, Losses: 12, GamesStarted: 20},
			Horoscope:  horo,
			Prediction: prediction.Prediction{WinProbability: 0.12, Pick: "L", Confidence: "The stars are screaming"},
		})
		assertFits(t, "game day", g.Text)
		if !strings.HasPrefix(g.Text, "⚾ Rockies ") || !strings.Contains(g.Text, "\n🔮 The stars are screaming a Rockies defeat (88%)") {
			t.Fatalf("lost the facts:\n%s", g.Text)
		}

		o := FormatOffDay(OffDayPost{Record: &mlb.TeamRecord{Wins: 58, Losses: 104, WinningPercentage: 0.358, RunDifferential: -201, StreakCode: "L11"}, Horoscope: horo})
		assertFits(t, "off day", o.Text)
		if !strings.HasPrefix(o.Text, "⚾ No Rockies game today") || !strings.Contains(o.Text, "Run Diff: -201") {
			t.Fatalf("lost the facts:\n%s", o.Text)
		}

		a := FormatAdoptedGame(AdoptedGame{
			TeamName: "Toronto Blue Jays", Opponent: "Los Angeles Dodgers", SeriesShort: "World Series Game 7", SeriesResult: "Series tied 3-3",
			GameTime: "6:08 PM MDT", Venue: "Rogers Centre", Pitcher: "Christopher Montgomery-Hernandez (2.95 ERA)",
			Prediction: prediction.Prediction{WinProbability: 0.52, Pick: "W", Confidence: "A cosmic coin flip"}, HoroscopeText: realHoroscopes[0],
		})
		assertFits(t, "adopted game", a.Text)
		if !strings.Contains(a.Text, "a Blue Jays victory (52%)") {
			t.Fatalf("lost the prediction:\n%s", a.Text)
		}
	}
}

func TestFormatFollowUp_Deterministic(t *testing.T) {
	f := FollowUp{Outcome: "Rockies L", Score: "Colorado Rockies 2-9 Los Angeles Dodgers", Correct: true, Record: "70/120 correct predictions this season"}
	if FormatFollowUp(f) != FormatFollowUp(f) {
		t.Error("same game should read the same")
	}
	assertFits(t, "follow-up", FormatFollowUp(f))
}
