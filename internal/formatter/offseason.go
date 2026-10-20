package formatter

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// MaxPostLength is Bluesky's post limit in graphemes. Runes are always >=
// graphemes, so limiting runes is conservative.
const MaxPostLength = 300

// Fit trims text to max runes, cutting at a line or word boundary.
func Fit(text string, max int) string {
	r := []rune(text)
	if len(r) <= max {
		return text
	}
	cut := string(r[:max-1])
	if i := strings.LastIndex(cut, "\n"); i > len(cut)/2 {
		cut = cut[:i]
	} else if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

// Nickname turns "San Diego Padres" into "Padres".
func Nickname(team string) string {
	for _, two := range []string{"Red Sox", "White Sox", "Blue Jays"} {
		if strings.HasSuffix(team, two) {
			return two
		}
	}
	if i := strings.LastIndex(team, " "); i >= 0 {
		return team[i+1:]
	}
	return team
}

func pct(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }

func shortDate(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.Format("Jan 2")
}

// ── Report card ──────────────────────────────────────────────────────

var factorNames = map[string]string{
	"winRate":  "Season win rate",
	"pitcher":  "Pitching matchup",
	"h2h":      "Head-to-head",
	"homeAway": "Home/away split",
	"momentum": "Streak momentum",
	"stars":    "🔮 The horoscope",
}

var factorOrder = []string{"winRate", "pitcher", "h2h", "homeAway", "momentum", "stars"}

func weightOf(w prediction.Weights, f string) float64 {
	switch f {
	case "winRate":
		return w.WinRate
	case "pitcher":
		return w.Pitcher
	case "h2h":
		return w.H2H
	case "homeAway":
		return w.HomeAway
	case "momentum":
		return w.Momentum
	default:
		return w.Stars
	}
}

// ReportCard renders the end-of-season thread, one string per post.
func ReportCard(s prediction.SeasonSummary) []string {
	var posts []string

	acc := s.Accuracy
	verdict := "A coin would have beaten us. The coin has asked not to be contacted."
	switch {
	case acc >= 0.60:
		verdict = "Better than the lineup. Better than most of the front office."
	case acc >= 0.50:
		verdict = "Better than a coin flip. Barely. The coin is filing an appeal."
	}
	pessimism := ""
	if s.PickedLosses > s.PickedWins {
		pessimism = fmt.Sprintf("\n📉 Picked them to lose %d times. Pessimism: a lifestyle.", s.PickedLosses)
	}
	posts = append(posts, fmt.Sprintf(
		"📜 The %d Rockiscope Report Card\n\n⚾ Rockies: %d-%d. The stars watched every one. Some twice, for therapy.\n🔮 Predictions: %d/%d correct (%s)%s\n\n%s 🧵",
		s.Season, s.RockiesWins, s.RockiesLosses, s.Correct, s.Predictions, pct(acc), pessimism, verdict))

	type row struct {
		key string
		acc float64
	}
	var rows []row
	for _, f := range factorOrder {
		if s.FactorSamples[f] >= 5 {
			rows = append(rows, row{f, s.FactorAccuracy[f]})
		}
	}
	if len(rows) > 0 {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].acc > rows[j].acc })
		var b strings.Builder
		b.WriteString("📊 Which signals actually predicted Rockies baseball?\n\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "%s: %s\n", factorNames[r.key], pct(r.acc))
		}
		if stars, ok := s.FactorSamples["stars"]; ok && stars >= 5 {
			if s.FactorAccuracy["stars"] > 0.5 {
				b.WriteString("\nThe stars beat a coin flip. Science is in shambles. Peer review is cancelled.")
			} else {
				b.WriteString("\nThe stars missed more than they hit. Still more consistent than the bullpen.")
			}
		}
		posts = append(posts, strings.TrimSpace(b.String()))
	}

	var b strings.Builder
	b.WriteString("🧠 What the model learned this year:\n\n")
	for _, f := range factorOrder {
		from, to := weightOf(s.StartWeights, f), weightOf(s.FinalWeights, f)
		arrow := "→"
		if math.Abs(to-from) < 0.005 {
			arrow = "＝"
		}
		fmt.Fprintf(&b, "%s: %s %s %s\n", factorNames[f], pct(from), arrow, pct(to))
	}
	if s.FinalWeights.Stars > s.StartWeights.Stars+0.005 {
		b.WriteString("\nThe machine is becoming a believer. It has started burning sage.")
	} else if s.FinalWeights.Stars < s.StartWeights.Stars-0.005 {
		b.WriteString("\nThe machine has lost its faith. We're praying for it, ironically.")
	}
	posts = append(posts, strings.TrimSpace(b.String()))

	b.Reset()
	if c := s.BestCall; c != nil {
		fmt.Fprintf(&b, "🎯 Best call: %s %s %s. Said %s at %s, got %s %d-%d.\n",
			shortDate(c.Date), homeAway(c.IsHome), Nickname(c.Opponent), c.Predicted, pct(c.PickConfidence()), c.Actual, c.RockiesScore, c.OppScore)
	}
	if m := s.WorstMiss; m != nil {
		fmt.Fprintf(&b, "💀 Worst miss: %s %s %s. Said %s at %s, got %s %d-%d.\n",
			shortDate(m.Date), homeAway(m.IsHome), Nickname(m.Opponent), m.Predicted, pct(m.PickConfidence()), m.Actual, m.RockiesScore, m.OppScore)
	}
	if s.LongestCorrectStreak > 0 {
		fmt.Fprintf(&b, "🔥 Longest correct streak: %d\n", s.LongestCorrectStreak)
	}
	b.WriteString("\nThe model goes into hibernation now. It'll be back. The bullpen remains under review. 🏔️")
	posts = append(posts, strings.TrimSpace(b.String()))

	for i := range posts {
		posts[i] = Fit(posts[i], MaxPostLength)
	}
	return posts
}

// ── Postseason adoption ──────────────────────────────────────────────

type Adoption struct {
	TeamName       string
	Reading        astro.RosterReading
	TeamsAudited   int
	RockiesRecord  string // "58-104", empty if unknown
	PreviousTeam   string // set when re-adopting after an elimination
	AdoptingItself bool   // the Rockies made the playoffs (hypothetically)
}

func FormatAdoption(a Adoption) string {
	if a.AdoptingItself {
		return Fit(fmt.Sprintf("🏔️ The Rockies are in the postseason. Rockiscope is adopting... the Rockies.\n\n%s\n\nSomeone pinch me. Gently. Then check the standings again.", a.Reading.Summary()), MaxPostLength)
	}
	var b strings.Builder
	if a.PreviousTeam != "" {
		fmt.Fprintf(&b, "🪦 The %s have been eliminated. Adoption dissolved. Their stuff is on the lawn.\n\n", Nickname(a.PreviousTeam))
		fmt.Fprintf(&b, "🔭 Next most compatible roster: the %s.\n", a.TeamName)
		fmt.Fprintf(&b, "%s\n♋ Dominant sign: %s\n\nWe've done this before. Please don't make it weird.", a.Reading.Summary(), a.Reading.Dominant)
		return Fit(b.String(), MaxPostLength)
	}
	b.WriteString("🍼 The Rockies are out")
	if a.RockiesRecord != "" {
		fmt.Fprintf(&b, " (%s, we checked)", a.RockiesRecord)
	}
	b.WriteString(". Rockiscope needs a team.\n\n")
	fmt.Fprintf(&b, "🔭 After a birth-chart audit of %d playoff rosters, we're adopting the %s.\n", a.TeamsAudited, a.TeamName)
	fmt.Fprintf(&b, "%s\n\nWe'll predict their games until they disappoint us too.", a.Reading.Summary())
	return Fit(b.String(), MaxPostLength)
}

type AdoptedGame struct {
	TeamName      string
	Opponent      string
	IsHome        bool
	SeriesShort   string // "NLDS Game 3"
	SeriesResult  string // "Series tied 1-1"
	GameTime      string
	Venue         string
	Pitcher       string // "Dylan Cease (3.12 ERA)", empty if TBD
	Prediction    prediction.Prediction
	HoroscopeText string
}

func FormatAdoptedGame(g AdoptedGame) Post {
	seed := seedOf(g.TeamName, g.SeriesShort, g.GameTime)
	text := fitting([]string{"roast", "kicker", "series"}, func(on map[string]bool) string {
		var b strings.Builder
		fmt.Fprintf(&b, "🍼 Adopted team watch: %s\n", g.SeriesShort)
		loc := "vs"
		if !g.IsHome {
			loc = "@"
		}
		fmt.Fprintf(&b, "⚾ %s %s %s\n", g.TeamName, loc, g.Opponent)
		fmt.Fprintf(&b, "🕐 %s at %s\n", g.GameTime, g.Venue)
		if g.SeriesResult != "" {
			line := "📈 " + g.SeriesResult
			if on["series"] {
				line = withQuip(line+".", pick(seriesQuips, seed))
			}
			b.WriteString(line + "\n")
		}
		if g.Pitcher != "" {
			fmt.Fprintf(&b, "🪖 %s\n", g.Pitcher)
		}
		if r := horoscopeRoast(g.HoroscopeText, seed, roastGameDay); on["roast"] && r != "" {
			b.WriteString(r + "\n")
		}
		b.WriteString("\n🔮 ")
		b.WriteString(TeamPrediction(g.Prediction, Nickname(g.TeamName)))
		if on["kicker"] {
			b.WriteString("\n" + predictionKicker(g.Prediction.WinProbability, seed))
		}
		return b.String()
	})
	return Post{Text: text, HoroscopeText: g.HoroscopeText, Prediction: g.Prediction}
}

var seriesQuips = []string{
	"The stars have read ahead.",
	"Every game is an elimination game if you believe hard enough.",
	"October is a state of mind.",
	"We are emotionally invested in a team we met last week.",
}

// TeamPrediction is Prediction.FormatPrediction for a team other than the Rockies.
func TeamPrediction(p prediction.Prediction, team string) string {
	return p.FormatFor(team)
}

var adoptedFollowUps = map[string][]string{
	"WW": {
		"✨ Called it. Our adopted child did us proud. Putting this one on the fridge.",
		"🍼 The stars said win. The kid listened. Better listener than our real kid.",
		"🔮 Correct pick AND a win? So this is what October feels like.",
		"🏅 A cosmic W for the adoptee. We're telling everyone at work.",
	},
	"WL": {
		"🫠 The stars doubted them. We'll keep that between us and the cosmos.",
		"🍀 Wrong pick, right result. Parenting is hard. Astrology is harder.",
		"🌈 We had no faith and were rewarded anyway. Don't tell the stars.",
		"🙈 Bet against our own adopted team. They won. We're in the doghouse.",
	},
	"LL": {
		"📉 Called it. We've seen this movie. We starred in it for years.",
		"🌑 The stars warned us. It feels like home.",
		"🏔️ A loss we saw coming. They're really one of us now.",
		"💀 Correct, and devastating. The adoption agency has questions.",
	},
	"LW": {
		"💔 The stars believed. That was the mistake.",
		"🫠 Wrong pick, worse result. Welcome to the family. Sorry.",
		"🌘 Mercury entered their bullpen in the middle innings.",
		"🧸 We believed in you, adopted team. That's on us now.",
	},
}

type AdoptedResult struct {
	TeamName     string
	Won          bool
	Correct      bool
	Score        string // "5-3"
	Opponent     string
	SeriesResult string
	PicksRecord  string // "3/5"
	Seed         int
}

func FormatAdoptedResult(r AdoptedResult) string {
	key := "L"
	if r.Won {
		key = "W"
	}
	if r.Correct {
		key += key
	} else if r.Won {
		key = "WL"
	} else {
		key = "LW"
	}
	msgs := adoptedFollowUps[key]
	msg := msgs[abs(r.Seed)%len(msgs)]
	wl := "L"
	if r.Won {
		wl = "W"
	}
	text := fmt.Sprintf("%s\n\n🏁 %s %s %s vs %s", msg, Nickname(r.TeamName), wl, r.Score, Nickname(r.Opponent))
	if r.SeriesResult != "" {
		text += "\n📈 " + r.SeriesResult
	}
	text += "\n🎯 Postseason picks: " + r.PicksRecord
	return Fit(text, MaxPostLength)
}

type Champion struct {
	Champion    string
	Adopted     []string // every team adopted this October, in order
	AdoptedWon  bool
	AdoptedOn   string // date the champion was adopted, when AdoptedWon
	PicksRecord string
}

func FormatChampion(c Champion) string {
	var b strings.Builder
	if c.AdoptedWon {
		fmt.Fprintf(&b, "🏆 The %s are World Series champions.\n\n", c.Champion)
		fmt.Fprintf(&b, "Rockiscope adopted them on %s. We're not saying the stars did this. We're not not saying it. The ring size is a 9.\n", shortDate(c.AdoptedOn))
	} else {
		fmt.Fprintf(&b, "🏆 The %s win the World Series.\n\n", c.Champion)
		if len(c.Adopted) > 0 {
			names := make([]string, len(c.Adopted))
			for i, a := range c.Adopted {
				names[i] = Nickname(a)
			}
			fmt.Fprintf(&b, "Rockiscope's adoptions this October: %s. All eliminated. The agency has revoked our license.\n", strings.Join(names, ", "))
		}
	}
	if c.PicksRecord != "" {
		fmt.Fprintf(&b, "🎯 Postseason picks: %s\n", c.PicksRecord)
	}
	b.WriteString("\n🔥 Hot stove season starts now. Every team is undefeated again, including ours. 🏔️")
	return Fit(b.String(), MaxPostLength)
}

// ── Hot stove ────────────────────────────────────────────────────────

var txnEmoji = map[string]string{
	"TR":  "🔁",
	"SFA": "✍️",
	"CLW": "🪝",
	"DES": "🚪",
	"REL": "👋",
	"DFA": "🕊️",
	"R5":  "🎲",
}

type TransactionPost struct {
	TypeCode    string
	Description string
	PlayerName  string
	BirthDate   string // empty if unknown
	Arriving    bool
}

func FormatTransaction(t TransactionPost) string {
	emoji := txnEmoji[t.TypeCode]
	if emoji == "" {
		emoji = "📋"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Hot stove: %s\n\n", emoji, t.Description)

	sign, ok := astro.SignFor(t.BirthDate)
	if !ok {
		b.WriteString("🔭 Birth chart unavailable. Proceeding on vibes. Vibes are cautiously suspicious.")
		return Fit(b.String(), MaxPostLength)
	}
	first := t.PlayerName
	if i := strings.Index(first, " "); i > 0 {
		first = first[:i]
	}
	fmt.Fprintf(&b, "%s %s is %s. %s\n", sign.Emoji(), first, withArticle(sign), sign.Trait())
	if t.Arriving {
		fmt.Fprintf(&b, "♋ Rockies compatibility: %s. %s", astro.CompatibilityLabel(sign.Compatibility()), pick(arrivalQuips[sign.Compatibility()], seedOf(t.Description)))
	} else {
		fmt.Fprintf(&b, "♋ Compatibility: no longer our problem. %s", pick(departureQuips, seedOf(t.Description)))
	}
	return Fit(b.String(), MaxPostLength)
}

// arrivalQuips react to a new Rockie by Cancer compatibility (0-3).
var arrivalQuips = [4][]string{
	{"The stars have filed a formal objection.", "We'll light a candle.", "Mercury is already drafting the trade request."},
	{"Couples therapy recommended.", "It could work. It won't, but it could.", "Proceed with mild cosmic caution."},
	{"A respectable match. Don't ruin it.", "The stars nod politely.", "Cautiously compatible."},
	{"Soulmates. The front office accidentally did astrology.", "Welcome home. Bring tissues.", "Destiny, signed through arbitration."},
}

var departureQuips = []string{
	"The stars wish them well. Mostly.",
	"Thank you for your service. And your ERA.",
	"Off to find themselves at sea level.",
	"May the thin air never haunt them.",
}

func FormatTransactionDigest(descriptions []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📋 Hot stove roundup, %d more moves. The front office has been busy, or at least typing:\n", len(descriptions))
	for _, d := range descriptions {
		fmt.Fprintf(&b, "\n• %s", d)
	}
	return Fit(b.String(), MaxPostLength)
}

// ── Opening Day countdown ────────────────────────────────────────────

var countdownQuips = []string{
	"Every team is undefeated right now. Even us. Especially us.",
	"The bullpen hasn't blown a lead in months. Personal best.",
	"Mercury will go retrograde before then. Plan accordingly.",
	"Somewhere, a reliever is throwing into a net and feeling great about it.",
	"Projection systems are warming up their 100-loss takes. Rude, but fair.",
	"Coors Field is under snow. So are expectations.",
	"Hope springs eternal. It also springs a leak by May.",
	"The stars are studying film. They've asked for a translator.",
	"Hibernation update: the model is dreaming of a .500 season.",
	"Saturn has been consulted about the rotation. Saturn sighed.",
}

var finalWeekQuips = []string{
	"Hope is a dangerous thing. We're doing it anyway.",
	"The stars are stretching. Hamstrings look questionable.",
	"Last chance to believe in something. Choose wisely.",
	"Clear your schedule. And your expectations.",
	"The crystals are charged. The bullpen is not.",
}

type Countdown struct {
	Days          int
	OpeningDay    string // "2006-01-02"
	Opponent      string // empty if unknown
	IsHome        bool
	SpringDays    int // days until spring games; <= 0 hides the line
	HoroscopeText string
}

func FormatCountdown(c Countdown) Post {
	var b strings.Builder
	switch c.Days {
	case 1:
		b.WriteString("🎆 Rockies Opening Day is TOMORROW.\n")
	default:
		fmt.Fprintf(&b, "⏳ %d days until Rockies Opening Day.\n", c.Days)
	}
	if c.Opponent != "" {
		loc := "vs"
		if !c.IsHome {
			loc = "@"
		}
		fmt.Fprintf(&b, "🗓️ %s %s %s\n", shortDate(c.OpeningDay), loc, c.Opponent)
	} else {
		fmt.Fprintf(&b, "🗓️ %s\n", shortDate(c.OpeningDay))
	}
	if c.SpringDays > 0 {
		fmt.Fprintf(&b, "🌵 Spring games start in %d days.\n", c.SpringDays)
	}

	quips := countdownQuips
	if c.Days <= 7 {
		quips = finalWeekQuips
	}
	fmt.Fprintf(&b, "\n%s", quips[c.Days%len(quips)])
	head := b.String()
	text := fitting([]string{"roast"}, func(on map[string]bool) string {
		t := head
		if c.HoroscopeText != "" {
			if r := horoscopeRoast(c.HoroscopeText, seedOf(c.OpeningDay, c.Days), roastCountdown); on["roast"] && r != "" {
				t += "\n\n" + r
			} else {
				t += "\n\n🔮 Today's reading is attached."
			}
		}
		return t
	})
	return Post{Text: text, HoroscopeText: c.HoroscopeText}
}

// ── Spring training ──────────────────────────────────────────────────

type SpringGame struct {
	Opponent      string
	IsHome        bool
	GameTime      string
	Venue         string
	SpringRecord  string // "5-7", empty before the first game
	Prediction    prediction.Prediction
	HoroscopeText string
}

func FormatSpringGame(g SpringGame) Post {
	seed := seedOf("spring", g.Opponent, g.GameTime, g.SpringRecord)
	text := fitting([]string{"roast", "kicker"}, func(on map[string]bool) string {
		var b strings.Builder
		b.WriteString("🌵 Spring Training. It doesn't count. Neither does this.\n")
		loc := "vs"
		if !g.IsHome {
			loc = "@"
		}
		fmt.Fprintf(&b, "⚾ Rockies %s %s\n", loc, g.Opponent)
		fmt.Fprintf(&b, "🕐 %s at %s\n", g.GameTime, g.Venue)
		if g.SpringRecord != "" {
			fmt.Fprintf(&b, "📊 Spring: %s\n", g.SpringRecord)
		}
		if r := horoscopeRoast(g.HoroscopeText, seed, roastGameDay); on["roast"] && r != "" {
			b.WriteString(r + "\n")
		}
		b.WriteString("\n🔮 ")
		b.WriteString(g.Prediction.FormatPrediction())
		if on["kicker"] {
			b.WriteString("\n" + pick(springKickers, seed))
		}
		return b.String()
	})
	return Post{Text: text, HoroscopeText: g.HoroscopeText, Prediction: g.Prediction}
}

var springKickers = []string{
	"Half the lineup will be wearing numbers in the 80s.",
	"The stars are also just getting loose.",
	"Results not valid in the regular season.",
	"Cactus League rules: nothing matters and the sun is out.",
}

// ── Season rollover ──────────────────────────────────────────────────

func FormatRollover(s prediction.SeasonSummary, newSeason int) string {
	text := fmt.Sprintf("🧹 New season, new model.\n\nThe %d predictions (%d/%d, %s) have been archived. Every weight is reset to factory settings. The stars have forgotten everything.\n\nSo have we. Mostly. Some things can't be unseen. %d starts now. 🏔️",
		s.Season, s.Correct, s.Predictions, pct(s.Accuracy), newSeason)
	return Fit(text, MaxPostLength)
}

func homeAway(isHome bool) string {
	if isHome {
		return "vs"
	}
	return "@"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
