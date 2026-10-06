package formatter

import (
	"strings"
	"testing"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/prediction"
)

func assertFits(t *testing.T, name, text string) {
	t.Helper()
	if n := len([]rune(text)); n > MaxPostLength {
		t.Errorf("%s is %d runes (max %d):\n%s", name, n, MaxPostLength, text)
	}
	if strings.TrimSpace(text) == "" {
		t.Errorf("%s is empty", name)
	}
}

func fullSummary() prediction.SeasonSummary {
	best := prediction.PredictionRecord{Date: "2026-05-02", Opponent: "Los Angeles Dodgers", IsHome: true, Predicted: "L", Actual: "L", WinProbability: 0.12, RockiesScore: 1, OppScore: 14}
	worst := prediction.PredictionRecord{Date: "2026-08-19", Opponent: "Arizona Diamondbacks", Predicted: "W", Actual: "L", WinProbability: 0.71, RockiesScore: 0, OppScore: 9}
	return prediction.SeasonSummary{
		Season: 2026, RockiesWins: 58, RockiesLosses: 104,
		Predictions: 150, Correct: 97, Accuracy: 0.647, PickedWins: 30, PickedLosses: 120,
		LongestCorrectStreak: 11,
		FactorAccuracy:       map[string]float64{"winRate": 0.64, "pitcher": 0.58, "h2h": 0.55, "homeAway": 0.6, "momentum": 0.57, "stars": 0.49},
		FactorSamples:        map[string]int{"winRate": 150, "pitcher": 120, "h2h": 90, "homeAway": 150, "momentum": 150, "stars": 150},
		StartWeights:         prediction.DefaultWeights(),
		FinalWeights:         prediction.Weights{WinRate: 0.11, Pitcher: 0.36, H2H: 0.03, HomeAway: 0.2, Momentum: 0.15, Stars: 0.15},
		BestCall:             &best, WorstMiss: &worst,
	}
}

func TestReportCard(t *testing.T) {
	posts := ReportCard(fullSummary())
	if len(posts) != 4 {
		t.Fatalf("expected 4-post thread, got %d", len(posts))
	}
	for i, p := range posts {
		assertFits(t, "report card post", p)
		_ = i
	}
	for _, want := range []string{"58-104", "97/150", "Pessimism: rewarded", "Better than the lineup"} {
		if !strings.Contains(posts[0], want) {
			t.Errorf("post 1 missing %q:\n%s", want, posts[0])
		}
	}
	if !strings.Contains(posts[1], "Season win rate: 64%") || !strings.Contains(posts[1], "Still more consistent than the bullpen") {
		t.Errorf("factor post:\n%s", posts[1])
	}
	if !strings.Contains(posts[2], "🔮 The horoscope: 10% → 15%") || !strings.Contains(posts[2], "becoming a believer") {
		t.Errorf("weights post:\n%s", posts[2])
	}
	if !strings.Contains(posts[3], "Best call: May 2 vs Dodgers. Said L at 88%") || !strings.Contains(posts[3], "streak: 11") {
		t.Errorf("highlights post:\n%s", posts[3])
	}
}

func TestReportCard_SparseSeason(t *testing.T) {
	posts := ReportCard(prediction.SeasonSummary{Season: 2026, StartWeights: prediction.DefaultWeights(), FinalWeights: prediction.DefaultWeights()})
	if len(posts) != 3 { // no factor post without samples
		t.Fatalf("got %d posts", len(posts))
	}
	if !strings.Contains(posts[0], "A coin would have beaten us") {
		t.Errorf("verdict:\n%s", posts[0])
	}
	for _, p := range posts {
		assertFits(t, "sparse report", p)
	}
}

func TestFormatAdoption(t *testing.T) {
	r := astro.RosterReading{Players: 26, Score: 1.73, WaterShare: 0.346, Dominant: "Scorpio"}
	first := FormatAdoption(Adoption{TeamName: "San Diego Padres", Reading: r, TeamsAudited: 8, RockiesRecord: "58-104"})
	assertFits(t, "adoption", first)
	for _, want := range []string{"58-104", "8 playoff rosters", "San Diego Padres", "35% water signs", "1.7/3"} {
		if !strings.Contains(first, want) {
			t.Errorf("missing %q:\n%s", want, first)
		}
	}

	again := FormatAdoption(Adoption{TeamName: "Milwaukee Brewers", Reading: r, PreviousTeam: "San Diego Padres"})
	assertFits(t, "re-adoption", again)
	if !strings.Contains(again, "Padres have been eliminated") || !strings.Contains(again, "Milwaukee Brewers") {
		t.Errorf("re-adoption:\n%s", again)
	}

	self := FormatAdoption(Adoption{TeamName: "Colorado Rockies", Reading: r, AdoptingItself: true})
	assertFits(t, "self adoption", self)
}

func TestFormatAdoptedGameAndResult(t *testing.T) {
	pred := prediction.Prediction{WinProbability: 0.58, Pick: "W", Confidence: "The stars lean toward"}
	post := FormatAdoptedGame(AdoptedGame{
		TeamName: "San Diego Padres", Opponent: "Milwaukee Brewers", IsHome: true,
		SeriesShort: "NLDS Game 3", SeriesResult: "MIL leads 2-0", GameTime: "7:38 PM MDT", Venue: "Petco Park",
		Pitcher: "Dylan Cease (3.12 ERA)", Prediction: pred, HoroscopeText: "x",
	})
	assertFits(t, "adopted game", post.Text)
	if !strings.Contains(post.Text, "Padres vs Milwaukee Brewers") || !strings.Contains(post.Text, "a Padres victory (58%)") {
		t.Errorf("adopted game:\n%s", post.Text)
	}
	if !strings.Contains(TeamPrediction(prediction.Prediction{WinProbability: 0.3, Pick: "L", Confidence: "c"}, "Padres"), "Padres defeat (70%)") {
		t.Error("loss phrasing wrong")
	}

	for _, won := range []bool{true, false} {
		for _, correct := range []bool{true, false} {
			for seed := 0; seed < 3; seed++ {
				text := FormatAdoptedResult(AdoptedResult{TeamName: "San Diego Padres", Opponent: "Milwaukee Brewers", Won: won, Correct: correct, Score: "4-3", SeriesResult: "MIL leads 2-1", PicksRecord: "2/3", Seed: seed})
				assertFits(t, "adopted result", text)
				if !strings.Contains(text, "Postseason picks: 2/3") {
					t.Errorf("result:\n%s", text)
				}
			}
		}
	}
}

func TestFormatChampion(t *testing.T) {
	won := FormatChampion(Champion{Champion: "San Diego Padres", AdoptedWon: true, AdoptedOn: "2026-10-06", PicksRecord: "9/14"})
	assertFits(t, "champion", won)
	if !strings.Contains(won, "adopted them on Oct 6") {
		t.Errorf("champion:\n%s", won)
	}
	lost := FormatChampion(Champion{Champion: "New York Yankees", Adopted: []string{"San Diego Padres", "Chicago White Sox"}, PicksRecord: "5/11"})
	assertFits(t, "champion lost", lost)
	if !strings.Contains(lost, "Padres, White Sox. All eliminated") {
		t.Errorf("champion:\n%s", lost)
	}
}

func TestFormatTransaction(t *testing.T) {
	in := FormatTransaction(TransactionPost{TypeCode: "TR", Description: "Boston Red Sox traded LHP Brennan Bernardino to Colorado Rockies for CF Braiden Ward.", PlayerName: "Brennan Bernardino", BirthDate: "1992-01-15", Arriving: true})
	assertFits(t, "txn", in)
	for _, want := range []string{"🔁 Hot stove:", "Brennan is a Capricorn", "Rockies compatibility: it's complicated"} {
		if !strings.Contains(in, want) {
			t.Errorf("missing %q:\n%s", want, in)
		}
	}
	out := FormatTransaction(TransactionPost{TypeCode: "DFA", Description: "RHP Germán Márquez elected free agency.", PlayerName: "Germán Márquez", BirthDate: "1995-02-22"})
	if !strings.Contains(out, "no longer our problem") || !strings.Contains(out, "Pisces") {
		t.Errorf("departure:\n%s", out)
	}
	unknown := FormatTransaction(TransactionPost{TypeCode: "ZZZ", Description: "Something happened."})
	if !strings.Contains(unknown, "Proceeding on vibes") || !strings.HasPrefix(unknown, "📋") {
		t.Errorf("unknown:\n%s", unknown)
	}

	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, "Colorado Rockies claimed RHP Somebody Withaverylongname off waivers from Miami Marlins.")
	}
	assertFits(t, "digest", FormatTransactionDigest(many))
}

func TestFormatCountdown(t *testing.T) {
	for _, days := range []int{170, 30, 7, 2, 1} {
		p := FormatCountdown(Countdown{Days: days, OpeningDay: "2027-03-25", Opponent: "San Francisco Giants", IsHome: false, SpringDays: days - 34, HoroscopeText: "x"})
		assertFits(t, "countdown", p.Text)
		if days == 1 && !strings.Contains(p.Text, "TOMORROW") {
			t.Errorf("final day:\n%s", p.Text)
		}
		if days == 170 && (!strings.Contains(p.Text, "170 days") || !strings.Contains(p.Text, "Mar 25 @ San Francisco Giants") || !strings.Contains(p.Text, "Spring games start in 136 days")) {
			t.Errorf("countdown:\n%s", p.Text)
		}
	}
	noOpp := FormatCountdown(Countdown{Days: 40, OpeningDay: "2027-03-25"})
	if strings.Contains(noOpp.Text, "Spring") || strings.Contains(noOpp.Text, "reading is attached") {
		t.Errorf("countdown w/o extras:\n%s", noOpp.Text)
	}
}

func TestFormatSpringAndRollover(t *testing.T) {
	p := FormatSpringGame(SpringGame{Opponent: "Cleveland Guardians", IsHome: true, GameTime: "1:10 PM MST", Venue: "Salt River Fields at Talking Stick", SpringRecord: "5-7",
		Prediction: prediction.Prediction{WinProbability: 0.4, Pick: "L", Confidence: "The stars lean toward"}})
	assertFits(t, "spring", p.Text)
	if !strings.Contains(p.Text, "Neither does this") || !strings.Contains(p.Text, "Spring: 5-7") {
		t.Errorf("spring:\n%s", p.Text)
	}

	r := FormatRollover(fullSummary(), 2027)
	assertFits(t, "rollover", r)
	if !strings.Contains(r, "2026 predictions (97/150, 65%)") || !strings.Contains(r, "2027 starts now") {
		t.Errorf("rollover:\n%s", r)
	}
}

func TestFitAndNickname(t *testing.T) {
	long := strings.Repeat("word ", 100)
	got := Fit(long, 50)
	if n := len([]rune(got)); n > 50 || !strings.HasSuffix(got, "…") {
		t.Errorf("Fit = %q (%d)", got, n)
	}
	if Fit("short", 50) != "short" {
		t.Error("short text changed")
	}
	for in, want := range map[string]string{"Chicago White Sox": "White Sox", "Toronto Blue Jays": "Blue Jays", "San Diego Padres": "Padres", "Athletics": "Athletics"} {
		if got := Nickname(in); got != want {
			t.Errorf("Nickname(%s) = %s", in, got)
		}
	}
}
