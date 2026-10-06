package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
	"github.com/tlugger/rockiscope/internal/seasonstate"
)

const did = "did:plc:abc123"

func writeFixture(t *testing.T, path string, v interface{}) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.Marshal(v)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// fixtureDir: 2025 is archived, 2026 is live and in the postseason.
func fixtureDir(t *testing.T) string {
	dir := t.TempDir()

	archived := prediction.PredictionHistory{
		Current: prediction.Weights{WinRate: 0.3, Pitcher: 0.3, H2H: 0.1, HomeAway: 0.1, Momentum: 0.1, Stars: 0.1},
		Predictions: []prediction.PredictionRecord{
			{Date: "2025-04-01", Opponent: "Miami Marlins", Predicted: "W", Actual: "W", WinProbability: 0.6},
			{Date: "2025-04-02", Opponent: "Miami Marlins", Predicted: "W", Actual: "L", WinProbability: 0.55},
		},
	}
	writeFixture(t, filepath.Join(dir, "archive", "2025", "prediction_history.json"), archived)
	sum := prediction.SummarizeSeason(archived.Predictions, 2025, archived.Current)
	writeFixture(t, filepath.Join(dir, "archive", "2025", "summary.json"), sum)

	live := prediction.PredictionHistory{
		Current: prediction.DefaultWeights(),
		Predictions: []prediction.PredictionRecord{
			{Date: "2026-04-01", Opponent: "Los Angeles Dodgers", IsHome: true, Predicted: "L", Actual: "L", RockiesScore: 1, OppScore: 7,
				WinProbability: 0.3, PostURI: "at://" + did + "/app.bsky.feed.post/r1", Factors: prediction.FactorScores{Stars: 0.4}},
			{Date: "2026-04-02", Opponent: "Off Day", Predicted: "N/A"},
			{Date: "2026-04-03", Opponent: "Los Angeles Dodgers", Predicted: "L", Actual: "W", WinProbability: 0.4},
		},
	}
	writeFixture(t, filepath.Join(dir, "prediction_history.json"), live)

	st := seasonstate.New()
	st.Status = seasonstate.Status{Phase: "postseason", Season: 2026, UpdatedAt: "2026-10-06T16:00:00Z"}
	st.SeasonDates["2026"] = mlb.SeasonDates{Season: "2026", SpringStart: "2026-02-20", RegularStart: "2026-03-26"}
	st.SeasonDates["2027"] = mlb.SeasonDates{Season: "2027", SpringStart: "2027-02-19", RegularStart: "2027-03-25"}
	st.OpeningDays["2027"] = seasonstate.OpeningDay{Date: "2027-03-25", Opponent: "San Francisco Giants"}
	st.Readings["2026:135"] = astro.RosterReading{Players: 26, Score: 1.73, WaterShare: 0.42, Dominant: "Scorpio"}
	st.Readings["2026:158"] = astro.RosterReading{Players: 26, Score: 1.2, WaterShare: 0.2, Dominant: "Aries"}
	st.Readings["2025:111"] = astro.RosterReading{Players: 26, Score: 2}
	st.Adoptions = []seasonstate.Adoption{{Season: 2026, TeamID: 135, TeamName: "San Diego Padres", AdoptedOn: "2026-10-06"}}
	first := time.Date(2026, 10, 7, 1, 30, 0, 0, time.UTC)
	st.Picks = []seasonstate.Pick{
		{Kind: "postseason", Season: 2026, GamePk: 1, Date: "2026-10-06", TeamID: 135, TeamName: "San Diego Padres", Opponent: "Milwaukee Brewers",
			IsHome: true, Pick: "W", WinProbability: 0.64, Result: "W", TeamScore: 5, OppScore: 3, Series: "NLDS Game 3", SeriesResult: "MIL leads 2-1",
			PostURI: "at://" + did + "/app.bsky.feed.post/p1", FirstPitch: first},
		{Kind: "postseason", Season: 2026, GamePk: 2, Date: "2026-10-07", TeamID: 135, TeamName: "San Diego Padres", Opponent: "Milwaukee Brewers",
			Pick: "W", WinProbability: 0.55, FirstPitch: first.Add(24 * time.Hour)},
		{Kind: "spring", Season: 2026, GamePk: 3, Date: "2026-03-01", Opponent: "Cleveland Guardians", Pick: "L", Result: "L", FirstPitch: first.AddDate(0, -7, 0)},
		{Kind: "spring", Season: 2026, GamePk: 4, Date: "2026-03-02", Opponent: "Cleveland Guardians", Pick: "L", Result: "void", FirstPitch: first.AddDate(0, -7, 1)},
	}
	st.HotStove = []seasonstate.HotStoveEntry{
		{Season: 2025, Date: "2025-11-09", ID: 10, TypeCode: "TR", Description: "trade", Sign: "Cancer", Compatibility: 3, Arriving: true, PostURI: "at://" + did + "/app.bsky.feed.post/h1"},
		{Season: 2025, Date: "2025-11-12", ID: 11, TypeCode: "REL", Description: "release", Sign: "Leo", Compatibility: 1},
	}
	st.Champions["2025"] = "Los Angeles Dodgers"
	writeFixture(t, seasonstate.Path(dir), st)
	return dir
}

func get(t *testing.T, srv *Server, path string, v interface{}) int {
	t.Helper()
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code == 200 && v != nil {
		if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
			t.Fatalf("%s: bad JSON: %v\n%s", path, err, w.Body.String())
		}
	}
	return w.Code
}

func testServer(dir string) *Server {
	srv := NewServer(dir, testLogger())
	srv.now = func() time.Time { return time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC) }
	return srv
}

func TestAPISeasons(t *testing.T) {
	var seasons []seasonInfo
	if code := get(t, testServer(fixtureDir(t)), "/api/seasons", &seasons); code != 200 {
		t.Fatalf("code %d", code)
	}
	if len(seasons) != 2 || seasons[0].Season != 2026 || seasons[1].Season != 2025 {
		t.Fatalf("seasons = %+v", seasons)
	}
	live, old := seasons[0], seasons[1]
	if !live.Live || live.Archived || live.Summary == nil || live.Summary.Predictions != 2 {
		t.Errorf("live season: %+v", live)
	}
	if live.Postseason == nil || live.Postseason.Picks != 1 || live.Postseason.Pending != 1 || len(live.Postseason.Adopted) != 1 {
		t.Errorf("postseason summary: %+v", live.Postseason)
	}
	if live.Spring == nil || live.Spring.Picks != 1 || live.Spring.Correct != 1 {
		t.Errorf("spring summary: %+v", live.Spring)
	}
	if old.Live || !old.Archived || old.Summary == nil || old.Summary.Correct != 1 || old.HotStoveMoves != 2 || old.Champion != "Los Angeles Dodgers" {
		t.Errorf("archived season: %+v", old)
	}
}

func TestAPISeason_Live(t *testing.T) {
	var d seasonDetail
	if code := get(t, testServer(fixtureDir(t)), "/api/season/2026", &d); code != 200 {
		t.Fatalf("code %d", code)
	}
	if d.Phase != "postseason" || d.Weights == nil || *d.Weights != prediction.DefaultWeights() {
		t.Errorf("phase/weights: %q %+v", d.Phase, d.Weights)
	}
	if len(d.Regular) != 2 || d.Regular[0].TeamScore != 1 || d.Regular[0].Team != "Colorado Rockies" {
		t.Errorf("regular games (off day must be excluded): %+v", d.Regular)
	}
	if d.Regular[0].PostURL != "https://bsky.app/profile/"+did+"/post/r1" {
		t.Errorf("post url = %q", d.Regular[0].PostURL)
	}
	ps := d.Postseason
	if len(ps.Audits) != 2 || ps.Audits[0].TeamID != 135 || !ps.Audits[0].Adopted || ps.Audits[1].Team != "Milwaukee Brewers" || ps.Audits[1].Adopted {
		t.Errorf("audits: %+v", ps.Audits)
	}
	if len(ps.Games) != 2 || ps.Games[0].Series != "NLDS Game 3" || ps.Games[0].Actual != "W" || ps.Games[1].Actual != "" {
		t.Errorf("postseason games: %+v", ps.Games)
	}
	if len(d.Spring) != 1 {
		t.Errorf("void spring game should be excluded: %+v", d.Spring)
	}
	if len(d.HotStove) != 0 {
		t.Errorf("2026 has no hot stove yet: %+v", d.HotStove)
	}
}

func TestAPISeason_Archived(t *testing.T) {
	var d seasonDetail
	if code := get(t, testServer(fixtureDir(t)), "/api/season/2025", &d); code != 200 {
		t.Fatalf("code %d", code)
	}
	if len(d.Regular) != 2 || d.Weights == nil || d.Weights.WinRate != 0.3 || d.Phase != "" {
		t.Errorf("archived regular: %+v weights %+v", d.Regular, d.Weights)
	}
	if len(d.HotStove) != 2 || d.HotStove[0].ID != 11 || d.HotStove[1].SignEmoji != "♋" || d.HotStove[1].CompatibilityLabel != "soulmates" || d.HotStove[0].CompatibilityLabel != "" {
		t.Errorf("hot stove (newest first, labels only for arrivals): %+v", d.HotStove)
	}
	if d.HotStove[1].PostURL == "" {
		t.Error("missing hot stove post link")
	}
	if len(d.Postseason.Audits) != 1 || d.Postseason.Champion != "Los Angeles Dodgers" {
		t.Errorf("postseason: %+v", d.Postseason)
	}
}

func TestAPISeason_BadRequests(t *testing.T) {
	srv := testServer(fixtureDir(t))
	if code := get(t, srv, "/api/season/1999", nil); code != 404 {
		t.Errorf("unknown season: %d", code)
	}
	if code := get(t, srv, "/api/season/abc", nil); code != 400 {
		t.Errorf("bad season: %d", code)
	}
}

func TestAPIStatus(t *testing.T) {
	var s statusView
	if code := get(t, testServer(fixtureDir(t)), "/api/status", &s); code != 200 {
		t.Fatalf("code %d", code)
	}
	if s.Phase != "postseason" || s.Season != 2026 || s.Today != "2026-10-06" {
		t.Errorf("status: %+v", s)
	}
	if s.Countdown == nil || s.Countdown.Days != 170 || s.Countdown.Opponent != "San Francisco Giants" || s.Countdown.SpringDays != 136 {
		t.Errorf("countdown: %+v", s.Countdown)
	}
	if s.Adoption == nil || s.Adoption.TeamName != "San Diego Padres" || s.LastPick == nil || s.LastPick.Date != "2026-10-07" {
		t.Errorf("adoption: %+v last %+v", s.Adoption, s.LastPick)
	}
	if s.Postseason == nil || s.Postseason.Correct != 1 || s.Rockies == nil || s.LatestMove == nil || s.LatestMove.ID != 11 {
		t.Errorf("summaries: %+v %+v %+v", s.Postseason, s.Rockies, s.LatestMove)
	}
}

func TestAPI_EmptyDataDir(t *testing.T) {
	for _, dir := range []string{t.TempDir(), ""} {
		srv := testServer(dir)
		var seasons []seasonInfo
		if code := get(t, srv, "/api/seasons", &seasons); code != 200 || len(seasons) != 0 {
			t.Errorf("seasons: %d %+v", code, seasons)
		}
		var s statusView
		if code := get(t, srv, "/api/status", &s); code != 200 || s.Phase != "" || s.Countdown != nil {
			t.Errorf("status: %d %+v", code, s)
		}
	}
}

func TestAPI_CorruptFilesFallBackToBackup(t *testing.T) {
	dir := fixtureDir(t)
	good, _ := os.ReadFile(seasonstate.Path(dir))
	os.WriteFile(seasonstate.Path(dir)+".bak", good, 0644)
	os.WriteFile(seasonstate.Path(dir), []byte(`{"status":`), 0644)

	var s statusView
	if code := get(t, testServer(dir), "/api/status", &s); code != 200 || s.Phase != "postseason" {
		t.Errorf("status after corruption: %d %+v", code, s)
	}
}

func TestPostURL(t *testing.T) {
	cases := map[string]string{
		"at://did:plc:x/app.bsky.feed.post/abc": "https://bsky.app/profile/did:plc:x/post/abc",
		"at://dry-run/app.bsky.feed.post/1":     "",
		"at://mock/post/1":                      "",
		"":                                      "",
	}
	for in, want := range cases {
		if got := postURL(in); got != want {
			t.Errorf("postURL(%q) = %q, want %q", in, got, want)
		}
	}
}
