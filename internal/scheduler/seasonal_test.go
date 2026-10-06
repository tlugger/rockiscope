package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// ── fakes ────────────────────────────────────────────────────────────

type mockSeason struct {
	dates    map[int]*mlb.SeasonDates
	games    []mlb.PostseasonGame
	gamesErr error
	rosters  map[int][]mlb.Person
	people   []mlb.Person
	txns     []mlb.Transaction
	records  map[int]*mlb.TeamRecord
	opening  []*mlb.Game
	panicky  bool
	calls    map[string]int
}

func (m *mockSeason) count(name string) {
	if m.calls == nil {
		m.calls = map[string]int{}
	}
	m.calls[name]++
}

func (m *mockSeason) GetSeasonDates(year int) (*mlb.SeasonDates, error) {
	m.count("dates")
	if m.panicky {
		panic("boom")
	}
	if d, ok := m.dates[year]; ok {
		return d, nil
	}
	return nil, errors.New("not published")
}
func (m *mockSeason) GetPostseasonGames(int) ([]mlb.PostseasonGame, error) {
	m.count("postseason")
	return m.games, m.gamesErr
}
func (m *mockSeason) GetRoster(id int) ([]mlb.Person, error) {
	m.count("roster")
	if r, ok := m.rosters[id]; ok {
		return r, nil
	}
	return nil, errors.New("no roster")
}
func (m *mockSeason) GetPeople([]int) ([]mlb.Person, error) { m.count("people"); return m.people, nil }
func (m *mockSeason) GetTransactions(string, string) ([]mlb.Transaction, error) {
	m.count("txns")
	return m.txns, nil
}
func (m *mockSeason) GetTeamRecordFor(id, _ int) (*mlb.TeamRecord, error) {
	if r, ok := m.records[id]; ok {
		return r, nil
	}
	return nil, errors.New("no record")
}
func (m *mockSeason) GetRockiesGamesBetween(string, string, string) ([]*mlb.Game, error) {
	m.count("opening")
	return m.opening, nil
}

type sentPost struct {
	text, parent, root string
	image              bool
}

// recordingPoster records threading and can be told to fail.
type recordingPoster struct {
	sent     []sentPost
	failNext int
}

func (p *recordingPoster) send(text string, img *bluesky.ImageData, parent, root string) (*bluesky.PostRef, error) {
	if p.failNext > 0 {
		p.failNext--
		return nil, errors.New("bluesky is down")
	}
	p.sent = append(p.sent, sentPost{text, parent, root, img != nil})
	return &bluesky.PostRef{URI: fmt.Sprintf("at://test/%d", len(p.sent))}, nil
}
func (p *recordingPoster) Post(text string, img *bluesky.ImageData) (*bluesky.PostRef, error) {
	return p.send(text, img, "", "")
}
func (p *recordingPoster) Reply(text string, img *bluesky.ImageData, parent, root string) (*bluesky.PostRef, error) {
	return p.send(text, img, parent, root)
}
func (p *recordingPoster) find(substr string) []sentPost {
	var out []sentPost
	for _, s := range p.sent {
		if strings.Contains(s.text, substr) {
			out = append(out, s)
		}
	}
	return out
}

func denverAt(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, mlb.DenverLocation())
}

type harness struct {
	now    time.Time
	season *mockSeason
	mlb    *mockMLB
	poster *recordingPoster
	dir    string
	hist   *prediction.PredictionHistory
}

func (h *harness) scheduler() *Scheduler {
	s := &Scheduler{
		mlb:         h.mlb,
		season:      h.season,
		horoscope:   &mockHoroscope{horo: &horoscope.Horoscope{Sign: "cancer", Text: "Trust the process. Or don't."}},
		poster:      h.poster,
		predHistory: h.hist,
		now:         func() time.Time { return h.now },
		sleep:       func(time.Duration) {},
		logger:      testLogger(),
		dataDir:     h.dir,
	}
	s.loadSeasonState()
	return s
}

func newHarness(t *testing.T) *harness {
	return &harness{
		season: &mockSeason{records: map[int]*mlb.TeamRecord{}},
		mlb:    &mockMLB{},
		poster: &recordingPoster{},
		dir:    t.TempDir(),
		hist:   &prediction.PredictionHistory{Current: prediction.DefaultWeights()},
	}
}

func seasonRecords2026() []prediction.PredictionRecord {
	var recs []prediction.PredictionRecord
	for i := 0; i < 12; i++ {
		actual := "L"
		if i%4 == 0 {
			actual = "W"
		}
		recs = append(recs, prediction.PredictionRecord{
			Date: fmt.Sprintf("2026-06-%02d", i+1), Opponent: "Los Angeles Dodgers", IsHome: i%2 == 0,
			Predicted: "L", Actual: actual, WinProbability: 0.3, GamePK: 1000 + i,
			Factors: prediction.FactorScores{WinRate: 0.36, Stars: 0.6, HomeAway: 0.4, Momentum: 0.3},
		})
	}
	return recs
}

func team(id int, name string) mlb.PostseasonTeam { return mlb.PostseasonTeam{ID: id, Name: name} }

func seriesGame(pk int, gameType string, n, total int, at time.Time, away, home mlb.PostseasonTeam, winner int) mlb.PostseasonGame {
	g := mlb.PostseasonGame{
		GamePk: pk, GameType: gameType, OfficialDate: at.Format("2006-01-02"), GameDateTime: at,
		Status: "Preview", DetailedState: "Scheduled", Venue: "Somewhere Park",
		SeriesShort: fmt.Sprintf("%s Game %d", gameType, n), SeriesGameNumber: n, GamesInSeries: total,
		Away: away, Home: home,
	}
	if winner != 0 {
		g.Status, g.DetailedState = "Final", "Final"
		if winner == away.ID {
			g.Away.IsWinner, g.Away.Score, g.Home.Score = true, 5, 3
		} else {
			g.Home.IsWinner, g.Home.Score, g.Away.Score = true, 5, 3
		}
		g.SeriesResult = "series result"
	}
	return g
}

func birthdays(dates ...string) []mlb.Person {
	var out []mlb.Person
	for i, d := range dates {
		out = append(out, mlb.Person{ID: i + 1, BirthDate: d})
	}
	return out
}

var (
	padres  = team(135, "San Diego Padres")
	brewers = team(158, "Milwaukee Brewers")
	dodgers = team(119, "Los Angeles Dodgers")
	braves  = team(144, "Atlanta Braves")
)

// ── phase ────────────────────────────────────────────────────────────

func TestClassifyPhase(t *testing.T) {
	y26 := &mlb.SeasonDates{Season: "2026", SpringStart: "2026-02-20", RegularStart: "2026-03-26", RegularEnd: "2026-09-27", PostEnd: "2026-10-31"}
	y27 := &mlb.SeasonDates{Season: "2027", SpringStart: "2027-02-19", RegularStart: "2027-03-25"}

	cases := []struct {
		today   string
		crowned bool
		opener  string
		want    Phase
		season  int
	}{
		{"2026-01-15", false, "", PhaseOffseason, 2025},
		{"2026-02-20", false, "", PhaseSpring, 2026},
		{"2026-03-26", false, "", PhaseRegular, 2026},
		{"2026-03-26", false, "2026-03-30", PhaseSpring, 2026}, // league opened abroad, Rockies haven't
		{"2026-09-27", false, "", PhaseRegular, 2026},
		{"2026-10-06", false, "", PhasePostseason, 2026},
		{"2026-10-29", true, "", PhaseOffseason, 2026},
		{"2026-11-15", false, "", PhaseOffseason, 2026},
	}
	for _, c := range cases {
		next := y27
		got := classifyPhase(c.today, 2026, y26, next, c.opener, c.crowned)
		if got.Phase != c.want || got.Season != c.season {
			t.Errorf("%s crowned=%v: got %s/%d, want %s/%d", c.today, c.crowned, got.Phase, got.Season, c.want, c.season)
		}
	}
	if p := classifyPhase("2026-11-15", 2026, y26, y27, "", false); p.Upcoming != y27 {
		t.Error("offseason should count down to next season")
	}
}

func TestFallbackPhase(t *testing.T) {
	for date, want := range map[string]Phase{
		"2026-01-10": PhaseOffseason, "2026-03-01": PhaseSpring, "2026-07-04": PhaseRegular,
		"2026-10-10": PhasePostseason, "2026-12-01": PhaseOffseason,
	} {
		d, _ := time.Parse("2006-01-02", date)
		if got := fallbackPhase(d); got.Phase != want || got.Source != "fallback" {
			t.Errorf("%s: %+v", date, got)
		}
	}
}

func TestCurrentPhase_CachesAndFallsBack(t *testing.T) {
	h := newHarness(t)
	h.now = denverAt(2026, 10, 6, 10, 0)
	h.season.dates = map[int]*mlb.SeasonDates{
		2026: {Season: "2026", SpringStart: "2026-02-20", RegularStart: "2026-03-26", RegularEnd: "2026-09-27", PostEnd: "2026-10-31"},
	}
	s := h.scheduler()
	if p := s.CurrentPhase(); p.Phase != PhasePostseason || p.Source != "api" {
		t.Fatalf("got %s", p)
	}
	calls := h.season.calls["dates"]
	s.CurrentPhase()
	if h.season.calls["dates"] != calls {
		t.Error("calendar should be fetched at most once a day")
	}

	// API down tomorrow: cached calendar still works after a restart.
	h.season.dates = nil
	h.now = h.now.AddDate(0, 0, 1)
	if p := h.scheduler().CurrentPhase(); p.Phase != PhasePostseason || p.Source != "cache" {
		t.Errorf("got %s", p)
	}

	// No cache and no API: month-based guess.
	h2 := newHarness(t)
	h2.now = denverAt(2026, 12, 1, 10, 0)
	if p := h2.scheduler().CurrentPhase(); p.Phase != PhaseOffseason || p.Source != "fallback" {
		t.Errorf("got %s", p)
	}

	// No season provider: legacy behavior.
	legacy := &Scheduler{now: time.Now, logger: testLogger()}
	if legacy.CurrentPhase().Phase != PhaseRegular {
		t.Error("nil season provider must keep legacy regular-season behavior")
	}
}

// ── postseason ───────────────────────────────────────────────────────

func postseasonHarness(t *testing.T) *harness {
	h := newHarness(t)
	h.hist.Predictions = seasonRecords2026()
	h.season.records[mlb.RockiesID] = &mlb.TeamRecord{Wins: 58, Losses: 104}
	h.season.records[135] = &mlb.TeamRecord{Wins: 90, Losses: 72, WinningPercentage: 0.556, HomeWins: 50, HomeLosses: 31}
	h.season.rosters = map[int][]mlb.Person{
		135: birthdays("1990-07-01", "1990-11-01", "1990-03-01"), // all water: 3.0
		158: birthdays("1990-04-01", "1990-04-02", "1990-04-03"), // Aries: 0.0
		119: birthdays("1990-05-01", "1990-09-01", "1990-05-02"), // earth: 2.0
		144: birthdays("1990-08-01", "1990-08-02", "1990-08-03"), // Leo: 1.0
	}
	gameTime := denverAt(2026, 10, 6, 19, 30)
	h.season.games = []mlb.PostseasonGame{
		seriesGame(1, "D", 1, 5, gameTime.AddDate(0, 0, -3), padres, brewers, 158),
		seriesGame(2, "D", 2, 5, gameTime.AddDate(0, 0, -2), padres, brewers, 158),
		seriesGame(3, "D", 3, 5, gameTime, brewers, padres, 0),
		seriesGame(10, "D", 1, 5, gameTime.AddDate(0, 0, -3), braves, dodgers, 119),
		seriesGame(11, "D", 2, 5, gameTime.Add(-4*time.Hour), dodgers, braves, 0),
		{GamePk: 99, GameType: "W", Away: team(2711, "Lower Seed League Champion"), Home: team(2712, "Higher Seed League Champion"), Status: "Preview"},
	}
	return h
}

func TestPostseason_FullOctober(t *testing.T) {
	h := postseasonHarness(t)
	h.now = denverAt(2026, 10, 6, 10, 0)
	s := h.scheduler()
	p := PhaseInfo{Phase: PhasePostseason, Season: 2026}

	// Morning: report card thread + adoption; too early for the game.
	r := s.runSeasonalTasks(p)
	if err := r.err(); err != nil {
		t.Fatal(err)
	}
	report := h.poster.find("Rockiscope Report Card")
	if len(report) != 1 || !strings.Contains(report[0].text, "58-104") {
		t.Fatalf("report card root missing: %+v", h.poster.sent)
	}
	threadReplies := 0
	for _, sp := range h.poster.sent {
		if sp.root == "at://test/1" {
			threadReplies++
		}
	}
	if threadReplies < 2 {
		t.Errorf("report card should be a thread, got %d replies", threadReplies)
	}
	adopt := h.poster.find("we're adopting the San Diego Padres")
	if len(adopt) != 1 || !strings.Contains(adopt[0].text, "58-104") || !strings.Contains(adopt[0].text, "4 playoff rosters") {
		t.Fatalf("adoption post: %+v", adopt)
	}
	if len(h.poster.find("Adopted team watch")) != 0 {
		t.Error("predicted too early")
	}
	if want := denverAt(2026, 10, 6, 18, 30); !r.next.Equal(want) {
		t.Errorf("next wake %s, want %s", r.next, want)
	}
	rosterCalls := h.season.calls["roster"]

	// An hour before first pitch: prediction with image.
	h.now = denverAt(2026, 10, 6, 18, 31)
	r = s.runSeasonalTasks(p)
	pred := h.poster.find("Adopted team watch")
	if len(pred) != 1 || !pred[0].image || !strings.Contains(pred[0].text, "Padres vs Milwaukee Brewers") {
		t.Fatalf("prediction: %+v", pred)
	}
	if len(s.state().Picks) != 1 {
		t.Fatalf("pick not recorded")
	}
	if h.season.calls["roster"] != rosterCalls {
		t.Error("roster readings should be cached")
	}

	// Game ends (Padres win): threaded reply with running record.
	h.season.games[2] = seriesGame(3, "D", 3, 5, denverAt(2026, 10, 6, 19, 30), brewers, padres, 135)
	h.now = denverAt(2026, 10, 6, 22, 40)
	s.runSeasonalTasks(p)
	replies := h.poster.find("Postseason picks: ")
	if len(replies) != 1 || replies[0].parent != s.state().Picks[0].PostURI {
		t.Fatalf("result reply: %+v", replies)
	}
	if !strings.Contains(replies[0].text, "Padres W 5-3") {
		t.Errorf("reply text: %s", replies[0].text)
	}

	// Game 4: Brewers clinch. Next morning we re-adopt (Dodgers, earth signs).
	h.season.games = append(h.season.games, seriesGame(4, "D", 4, 5, denverAt(2026, 10, 7, 19, 30), brewers, padres, 158))
	h.now = denverAt(2026, 10, 8, 10, 0)
	s.runSeasonalTasks(p)
	re := h.poster.find("Padres have been eliminated")
	if len(re) != 1 || !strings.Contains(re[0].text, "Los Angeles Dodgers") {
		t.Fatalf("re-adoption: %+v", re)
	}
	if a := s.currentAdoption(2026); a == nil || a.TeamID != 119 {
		t.Fatalf("current adoption = %+v", a)
	}

	// World Series: Dodgers sweep. Champion post claims credit; phase flips.
	ws := denverAt(2026, 10, 24, 18, 0)
	for i := 0; i < 4; i++ {
		h.season.games = append(h.season.games, seriesGame(200+i, "W", i+1, 7, ws.AddDate(0, 0, i), team(147, "New York Yankees"), dodgers, 119))
	}
	h.now = denverAt(2026, 10, 28, 22, 30)
	s.runSeasonalTasks(p)
	champ := h.poster.find("World Series champions")
	if len(champ) != 1 || !strings.Contains(champ[0].text, "adopted them on Oct 8") {
		t.Fatalf("champion: %+v", champ)
	}
	if s.state().Champions["2026"] != "Los Angeles Dodgers" || s.state().HotStoveSince != "2026-10-28" {
		t.Errorf("champion state: %+v / %s", s.state().Champions, s.state().HotStoveSince)
	}

	// A restart replays nothing.
	total := len(h.poster.sent)
	s2 := h.scheduler()
	s2.runSeasonalTasks(PhaseInfo{Phase: PhaseOffseason, Season: 2026})
	if len(h.poster.sent) != total {
		for _, sp := range h.poster.sent[total:] {
			t.Errorf("duplicate after restart: %s", sp.text)
		}
	}
}

func TestPostseason_MissedPregameWindowIsSkipped(t *testing.T) {
	h := postseasonHarness(t)
	h.now = denverAt(2026, 10, 6, 20, 30) // Pi was down; game started an hour ago
	s := h.scheduler()
	s.runSeasonalTasks(PhaseInfo{Phase: PhasePostseason, Season: 2026})
	if len(h.poster.find("Adopted team watch")) != 0 {
		t.Error("must not predict a game already in progress")
	}
	if !s.done("ps:pred:3") {
		t.Error("missed game should be marked so it isn't retried")
	}
}

func TestPostseason_QuietHoursDeferChatter(t *testing.T) {
	h := postseasonHarness(t)
	h.now = denverAt(2026, 10, 6, 23, 30)
	s := h.scheduler()
	r := s.runSeasonalTasks(PhaseInfo{Phase: PhasePostseason, Season: 2026})
	if len(h.poster.sent) != 0 {
		t.Errorf("posted during quiet hours: %+v", h.poster.sent)
	}
	if want := denverAt(2026, 10, 7, 8, 0); !r.next.Equal(want) {
		t.Errorf("next wake %s, want %s", r.next, want)
	}
}

func TestPostseason_ResumesThreadAfterFailure(t *testing.T) {
	h := postseasonHarness(t)
	h.now = denverAt(2026, 10, 6, 10, 0)
	s := h.scheduler()
	p := PhaseInfo{Phase: PhasePostseason, Season: 2026}

	// First post goes out, then Bluesky falls over.
	h.poster.failNext = 0
	s.postOnce("report:2026:0", "pre-seeded root", nil, "", "") // simulate part 0 already posted before a crash
	h.poster.failNext = 5
	r := s.runSeasonalTasks(p)
	if r.err() == nil {
		t.Fatal("expected failure")
	}
	if d := s.seasonalTick(p); d > retryDelay {
		t.Errorf("failure should retry within %s, got %s", retryDelay, d)
	}

	// Restart, Bluesky healthy: thread resumes under the original root.
	h.poster.failNext = 0
	s = h.scheduler()
	s.runSeasonalTasks(p)
	if n := len(h.poster.find("pre-seeded root")); n != 1 {
		t.Errorf("root reposted %d times", n)
	}
	for _, sp := range h.poster.sent[1:] {
		if strings.Contains(sp.text, "What the model learned") && sp.root != "at://test/1" {
			t.Errorf("resumed thread under wrong root: %+v", sp)
		}
	}
	if !s.done("report:2026") {
		t.Error("report card should be complete")
	}
}

func TestPostseason_APIFailureRetriesSoon(t *testing.T) {
	h := postseasonHarness(t)
	h.season.gamesErr = errors.New("statsapi 503")
	h.now = denverAt(2026, 10, 6, 10, 0)
	s := h.scheduler()
	s.state().Done["report:2026"] = ""
	if d := s.seasonalTick(PhaseInfo{Phase: PhasePostseason, Season: 2026}); d != retryDelay {
		t.Errorf("sleep = %s, want %s", d, retryDelay)
	}
}

func TestAnalyzeBracket(t *testing.T) {
	at := denverAt(2026, 10, 1, 18, 0)
	b := analyzeBracket([]mlb.PostseasonGame{
		seriesGame(1, "F", 1, 3, at, padres, brewers, 135),
		seriesGame(2, "F", 2, 3, at, padres, brewers, 158),
		seriesGame(3, "F", 3, 3, at, padres, brewers, 0),
		seriesGame(4, "F", 1, 3, at, dodgers, braves, 119),
		seriesGame(5, "F", 2, 3, at, dodgers, braves, 119),
	})
	if !b.eliminated[144] || b.eliminated[135] || b.eliminated[158] || b.champion != 0 {
		t.Errorf("eliminated=%v champion=%d", b.eliminated, b.champion)
	}
	if alive := b.alive(); len(alive) != 3 {
		t.Errorf("alive = %v", alive)
	}
}

// ── daily cap & panics ───────────────────────────────────────────────

func TestPostOnce_DailyCap(t *testing.T) {
	h := newHarness(t)
	h.now = denverAt(2026, 10, 6, 12, 0)
	s := h.scheduler()
	s.state().PostsByDay["2026-10-06"] = maxPostsPerDay
	if _, err := s.postOnce("x", "hello", nil, "", ""); !errors.Is(err, errDailyCap) {
		t.Fatalf("err = %v", err)
	}
	if len(h.poster.sent) != 0 {
		t.Error("cap ignored")
	}
}

func TestIterate_RecoversFromPanic(t *testing.T) {
	h := newHarness(t)
	h.season.panicky = true
	h.now = denverAt(2026, 10, 6, 12, 0)
	if d := h.scheduler().iterate(); d != retryDelay {
		t.Errorf("sleep after panic = %s", d)
	}
}

// ── hot stove ────────────────────────────────────────────────────────

func TestHotStove(t *testing.T) {
	h := newHarness(t)
	h.now = denverAt(2026, 11, 10, 10, 5)
	h.season.txns = []mlb.Transaction{
		{ID: 2, Date: "2026-11-09", TypeCode: "TR", Description: "Boston Red Sox traded LHP A to Colorado Rockies for CF B.", PersonID: 502, PersonName: "Be Ta", ToTeamID: 111, FromTeamID: 115},
		{ID: 1, Date: "2026-11-09", TypeCode: "TR", Description: "Boston Red Sox traded LHP A to Colorado Rockies for CF B.", PersonID: 501, PersonName: "Al Pha", ToTeamID: 115, FromTeamID: 111},
		{ID: 3, Date: "2026-11-09", TypeCode: "SFA", Description: "Colorado Rockies signed free agent RHP C.", PersonID: 503, PersonName: "Ce Ce", ToTeamID: 115},
		{ID: 4, Date: "2026-11-09", TypeCode: "SFA", Description: "Colorado Rockies signed free agent C D to a minor league contract.", ToTeamID: 115},
		{ID: 5, Date: "2026-11-10", TypeCode: "CLW", Description: "Colorado Rockies claimed OF E off waivers.", PersonID: 505, ToTeamID: 115},
		{ID: 6, Date: "2026-11-10", TypeCode: "DES", Description: "Colorado Rockies designated 1B F for assignment.", PersonID: 506, ToTeamID: 115},
		{ID: 7, Date: "2026-11-10", TypeCode: "REL", Description: "Colorado Rockies released RHP G.", PersonID: 507},
		{ID: 8, Date: "2026-11-10", TypeCode: "ASG", Description: "C H assigned to Colorado Rockies."},
		{ID: 9, Date: "2026-10-20", TypeCode: "TR", Description: "Old news trade."},
	}
	h.season.people = []mlb.Person{{ID: 501, BirthDate: "1991-07-05"}}
	s := h.scheduler()
	s.state().HotStoveSince = "2026-10-29"

	r := &tickResult{now: h.now}
	s.hotStoveTask(r)
	if err := r.err(); err != nil {
		t.Fatal(err)
	}
	if len(h.poster.sent) != 4 {
		for _, sp := range h.poster.sent {
			t.Log(sp.text)
		}
		t.Fatalf("expected 3 moves + 1 digest, got %d posts", len(h.poster.sent))
	}
	if !strings.Contains(h.poster.sent[0].text, "Al is a Cancer") || !strings.Contains(h.poster.sent[0].text, "soulmates") {
		t.Errorf("trade post: %s", h.poster.sent[0].text)
	}
	if !strings.Contains(h.poster.sent[3].text, "2 more moves") || !strings.Contains(h.poster.sent[3].text, "released RHP G") {
		t.Errorf("digest: %s", h.poster.sent[3].text)
	}
	for _, sp := range h.poster.sent {
		if strings.Contains(sp.text, "minor league") || strings.Contains(sp.text, "assigned") || strings.Contains(sp.text, "Old news") {
			t.Errorf("noise posted: %s", sp.text)
		}
	}
	if want := denverAt(2026, 11, 10, 17, 0); !r.next.Equal(want) {
		t.Errorf("next check %s, want %s", r.next, want)
	}

	// Same slot again, and the evening slot after a restart: nothing new.
	s.hotStoveTask(&tickResult{now: h.now})
	h.now = denverAt(2026, 11, 10, 17, 5)
	h.scheduler().hotStoveTask(&tickResult{now: h.now})
	if len(h.poster.sent) != 4 {
		t.Errorf("reposted moves: %d posts", len(h.poster.sent))
	}
}

func TestHotStove_BeforeFirstSlotWaits(t *testing.T) {
	h := newHarness(t)
	h.now = denverAt(2026, 11, 10, 7, 0)
	r := &tickResult{now: h.now}
	h.scheduler().hotStoveTask(r)
	if h.season.calls["txns"] != 0 {
		t.Error("checked before the first slot")
	}
	if want := denverAt(2026, 11, 10, 10, 0); !r.next.Equal(want) {
		t.Errorf("next %s", r.next)
	}
}

// ── countdown ────────────────────────────────────────────────────────

func countdownHarness(t *testing.T) (*harness, PhaseInfo) {
	h := newHarness(t)
	h.season.opening = []*mlb.Game{{
		OfficialDate: "2027-03-25", GameDateTime: denverAt(2027, 3, 25, 14, 0), IsHome: false,
		HomeTeam: mlb.TeamInfo{ID: 137, Name: "San Francisco Giants"}, AwayTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
	}}
	return h, PhaseInfo{Phase: PhaseOffseason, Season: 2026,
		Upcoming: &mlb.SeasonDates{Season: "2027", SpringStart: "2027-02-19", RegularStart: "2027-03-25"}}
}

func TestCountdown_WeeklyThenDaily(t *testing.T) {
	h, p := countdownHarness(t)
	h.now = denverAt(2026, 11, 15, 9, 0) // Sunday, before 10
	s := h.scheduler()

	r := &tickResult{now: h.now}
	s.countdownTask(p, r)
	if len(h.poster.sent) != 0 || !r.next.Equal(denverAt(2026, 11, 15, 10, 0)) {
		t.Fatalf("early: posts=%d next=%s", len(h.poster.sent), r.next)
	}

	h.now = denverAt(2026, 11, 15, 10, 1)
	s.countdownTask(p, &tickResult{now: h.now})
	if len(h.poster.sent) != 1 || !h.poster.sent[0].image {
		t.Fatalf("posts = %+v", h.poster.sent)
	}
	txt := h.poster.sent[0].text
	if !strings.Contains(txt, "130 days until Rockies Opening Day") || !strings.Contains(txt, "Mar 25 @ San Francisco Giants") || !strings.Contains(txt, "Spring games start in 96 days") {
		t.Errorf("countdown: %s", txt)
	}

	// Monday: already posted this week; next is Sunday.
	h.now = denverAt(2026, 11, 16, 10, 30)
	r = &tickResult{now: h.now}
	s.countdownTask(p, r)
	if len(h.poster.sent) != 1 || !r.next.Equal(denverAt(2026, 11, 22, 10, 0)) {
		t.Errorf("monday: posts=%d next=%s", len(h.poster.sent), r.next)
	}
	if h.season.calls["opening"] != 1 {
		t.Error("opening day should be cached")
	}

	// Pi was down Sunday: Tuesday catches up once.
	h.now = denverAt(2026, 11, 24, 12, 0)
	s.countdownTask(p, &tickResult{now: h.now})
	s.countdownTask(p, &tickResult{now: h.now})
	if len(h.poster.sent) != 2 {
		t.Errorf("catch-up posts = %d", len(h.poster.sent))
	}

	// Final week: every day.
	sp := PhaseInfo{Phase: PhaseSpring, Season: 2027, Upcoming: p.Upcoming}
	for day := 20; day <= 24; day++ {
		h.now = denverAt(2027, 3, day, 10, 15)
		s.countdownTask(sp, &tickResult{now: h.now})
	}
	if len(h.poster.sent) != 7 || !strings.Contains(h.poster.sent[6].text, "TOMORROW") {
		t.Errorf("final week posts = %d, last: %s", len(h.poster.sent), h.poster.sent[len(h.poster.sent)-1].text)
	}
}

// ── spring ───────────────────────────────────────────────────────────

func TestSpring_PredictsAndSettles(t *testing.T) {
	h := newHarness(t)
	first := denverAt(2027, 3, 1, 13, 5)
	h.mlb.games = []*mlb.Game{{
		GamePk: 777, GameType: "S", OfficialDate: "2027-03-01", GameDateTime: first, Status: "Preview", DetailedState: "Scheduled",
		Venue: "Salt River Fields", IsHome: true,
		HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies", Wins: 3, Losses: 2},
		AwayTeam: mlb.TeamInfo{ID: 114, Name: "Cleveland Guardians"},
	}}
	h.now = first.Add(-50 * time.Minute)
	s := h.scheduler()

	s.springTask(2027, &tickResult{now: h.now})
	if len(h.poster.sent) != 1 || !strings.Contains(h.poster.sent[0].text, "Spring: 3-2") {
		t.Fatalf("spring post: %+v", h.poster.sent)
	}

	h.mlb.gamesSince = []mlb.GameResult{{GamePk: 777, Won: true, RockiesScore: 6, OppScore: 2, Opponent: "Cleveland Guardians"}}
	h.mlb.games[0].Status = "Final"
	h.now = first.Add(3 * time.Hour)
	s.springTask(2027, &tickResult{now: h.now})
	if len(h.poster.sent) != 2 || h.poster.sent[1].parent != "at://test/1" || !strings.Contains(h.poster.sent[1].text, "spring picks") {
		t.Fatalf("spring reply: %+v", h.poster.sent)
	}
	if h.hist.TotalCount() != 0 {
		t.Error("spring picks leaked into the real prediction history")
	}
}

func TestSpring_IgnoresNonSpringAndVoidsRainouts(t *testing.T) {
	h := newHarness(t)
	h.now = denverAt(2027, 3, 5, 12, 0)
	h.mlb.games = []*mlb.Game{{GamePk: 1, GameType: "R", GameDateTime: h.now.Add(30 * time.Minute), Status: "Preview", DetailedState: "Scheduled"}}
	s := h.scheduler()
	s.state().Picks = []pick{{Kind: pickSpring, Season: 2027, GamePk: 5, Date: "2027-03-01", FirstPitch: denverAt(2027, 3, 1, 13, 0), PostURI: "at://x"}}

	s.springTask(2027, &tickResult{now: h.now})
	if len(h.poster.sent) != 0 {
		t.Errorf("posted: %+v", h.poster.sent)
	}
	if s.state().Picks[0].Result != "void" {
		t.Errorf("rainout not voided: %+v", s.state().Picks[0])
	}
}

// ── rollover ─────────────────────────────────────────────────────────

func TestRollover_ArchivesResetsAndAnnouncesOnce(t *testing.T) {
	h := newHarness(t)
	h.hist.Predictions = seasonRecords2026()
	h.hist.Current = prediction.Weights{WinRate: 0.2, Pitcher: 0.4, H2H: 0.05, HomeAway: 0.15, Momentum: 0.1, Stars: 0.1}
	h.now = denverAt(2027, 2, 19, 12, 0)
	s := h.scheduler()

	s.rolloverTask(2027, &tickResult{now: h.now})
	if len(h.hist.Predictions) != 0 || h.hist.Current != prediction.DefaultWeights() {
		t.Fatalf("history not reset")
	}
	if _, err := os.Stat(filepath.Join(h.dir, "archive", "2026", "summary.json")); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
	if len(h.poster.sent) != 1 || !strings.Contains(h.poster.sent[0].text, "New season, new model") {
		t.Fatalf("announcement: %+v", h.poster.sent)
	}

	h.scheduler().rolloverTask(2027, &tickResult{now: h.now})
	if len(h.poster.sent) != 1 {
		t.Error("announced twice")
	}
}

func TestIterate_RegularSeasonCatchesMissedRollover(t *testing.T) {
	h := newHarness(t)
	h.hist.Predictions = seasonRecords2026()
	h.season.dates = map[int]*mlb.SeasonDates{
		2027: {Season: "2027", SpringStart: "2027-02-19", RegularStart: "2027-03-25", RegularEnd: "2027-09-26", PostEnd: "2027-10-31"},
	}
	h.mlb.record = &mlb.TeamRecord{}
	h.now = denverAt(2027, 4, 2, 12, 0)
	s := h.scheduler()
	s.lastPostDate = "2027-04-02"
	s.iterate()
	if len(h.hist.Predictions) != 0 {
		t.Errorf("rollover skipped in regular season: %d records left", len(h.hist.Predictions))
	}
}

func TestRunOnce_SeasonalPhaseIgnoresTimeGates(t *testing.T) {
	h := postseasonHarness(t)
	h.season.dates = map[int]*mlb.SeasonDates{
		2026: {Season: "2026", SpringStart: "2026-02-20", RegularStart: "2026-03-26", RegularEnd: "2026-09-27", PostEnd: "2026-10-31"},
	}
	h.now = denverAt(2026, 10, 6, 6, 0) // quiet hours, game 13h away
	s := h.scheduler()
	if err := s.RunOnce(); err != nil {
		t.Fatal(err)
	}
	if len(h.poster.find("Adopted team watch")) != 1 || len(h.poster.find("Report Card")) != 1 {
		t.Errorf("forced run should post everything due today: %d posts", len(h.poster.sent))
	}
}

func TestSeasonState_PersistsAndRecovers(t *testing.T) {
	h := newHarness(t)
	h.now = denverAt(2026, 11, 1, 12, 0)
	s := h.scheduler()
	s.markDone("k", "at://1")

	// Corrupt the main file; the backup from the earlier save is used.
	s.markDone("k2", "at://2")
	os.WriteFile(filepath.Join(h.dir, seasonStateFileName), []byte(`{"done": {`), 0644)
	if !h.scheduler().done("k") {
		t.Error("state not recovered from backup")
	}

	// Dry runs never write.
	h2 := newHarness(t)
	h2.now = h.now
	d := h2.scheduler()
	d.dryRun = true
	d.markDone("k", "")
	if _, err := os.Stat(filepath.Join(h2.dir, seasonStateFileName)); !os.IsNotExist(err) {
		t.Error("dry run wrote state")
	}
}

func TestIterate_WaitsForClockSync(t *testing.T) {
	h := newHarness(t)
	h.now = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if d := h.scheduler().iterate(); d != time.Minute || len(h.season.calls) != 0 {
		t.Errorf("acted on an unsynced clock: sleep=%s calls=%v", d, h.season.calls)
	}
}
