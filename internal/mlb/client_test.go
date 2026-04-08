package mlb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func fixtureServer(t *testing.T, fixturePath string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", fixturePath, err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
}

func testClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	c := NewClient(&http.Client{})
	c.now = func() time.Time {
		return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	}
	return c
}

// patchBaseURL temporarily replaces the baseURL for testing against httptest servers.
// Returns a cleanup function to restore the original.
func patchBaseURL(url string) func() {
	// We can't reassign a const, so we use a different approach:
	// The client methods use baseURL directly, so we need to override at the HTTP level.
	// Instead, we'll use a custom transport that rewrites URLs.
	return func() {}
}

func TestGetTodayGame(t *testing.T) {
	ts := fixtureServer(t, "../../testdata/schedule.json")
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		teamID:     RockiesID,
		now:        func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) },
	}

	// Override getJSON to use test server
	game, err := c.getGameFromURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if game == nil {
		t.Fatal("expected a game, got nil")
	}

	if game.Venue != "Coors Field" {
		t.Errorf("venue = %q, want %q", game.Venue, "Coors Field")
	}

	if !game.IsHome {
		t.Error("expected Rockies to be home team")
	}

	if game.Opponent().Name != "Houston Astros" {
		t.Errorf("opponent = %q, want %q", game.Opponent().Name, "Houston Astros")
	}

	if game.HomeTeam.ProbablePitcher == nil {
		t.Fatal("expected home probable pitcher")
	}
	if game.HomeTeam.ProbablePitcher.FullName != "Michael Lorenzen" {
		t.Errorf("home pitcher = %q, want %q", game.HomeTeam.ProbablePitcher.FullName, "Michael Lorenzen")
	}

	if game.AwayTeam.ProbablePitcher == nil {
		t.Fatal("expected away probable pitcher")
	}
	if game.AwayTeam.ProbablePitcher.FullName != "Framber Valdez" {
		t.Errorf("away pitcher = %q, want %q", game.AwayTeam.ProbablePitcher.FullName, "Framber Valdez")
	}

	if game.RockiesTeam().Wins != 5 || game.RockiesTeam().Losses != 6 {
		t.Errorf("rockies record = %d-%d, want 5-6", game.RockiesTeam().Wins, game.RockiesTeam().Losses)
	}
}

func TestGetTodayGame_OffDay(t *testing.T) {
	ts := fixtureServer(t, "../../testdata/schedule_offday.json")
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		teamID:     RockiesID,
		now:        func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) },
	}

	game, err := c.getGameFromURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game != nil {
		t.Fatalf("expected nil game on off-day, got %+v", game)
	}
}

func TestGetTeamRecord(t *testing.T) {
	ts := fixtureServer(t, "../../testdata/standings.json")
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		teamID:     RockiesID,
		now:        func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) },
	}

	rec, err := c.getTeamRecordFromURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Wins != 5 || rec.Losses != 6 {
		t.Errorf("record = %d-%d, want 5-6", rec.Wins, rec.Losses)
	}
	if rec.RunDifferential != 5 {
		t.Errorf("run diff = %d, want 5", rec.RunDifferential)
	}
	if rec.StreakCode != "W2" {
		t.Errorf("streak = %q, want %q", rec.StreakCode, "W2")
	}
	if rec.HomeWins != 3 || rec.HomeLosses != 2 {
		t.Errorf("home = %d-%d, want 3-2", rec.HomeWins, rec.HomeLosses)
	}
	if rec.AwayWins != 2 || rec.AwayLosses != 4 {
		t.Errorf("away = %d-%d, want 2-4", rec.AwayWins, rec.AwayLosses)
	}
}

func TestGetPitcherStats(t *testing.T) {
	ts := fixtureServer(t, "../../testdata/pitcher_stats.json")
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		teamID:     RockiesID,
		now:        func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) },
	}

	stats, err := c.getPitcherStatsFromURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.FullName != "Michael Lorenzen" {
		t.Errorf("name = %q, want %q", stats.FullName, "Michael Lorenzen")
	}
	if stats.ERA != 9.0 {
		t.Errorf("ERA = %f, want 9.0", stats.ERA)
	}
	if stats.WHIP != 2.31 {
		t.Errorf("WHIP = %f, want 2.31", stats.WHIP)
	}
	if stats.Wins != 0 || stats.Losses != 1 {
		t.Errorf("record = %d-%d, want 0-1", stats.Wins, stats.Losses)
	}
	if stats.StrikeOuts != 10 {
		t.Errorf("strikeouts = %d, want 10", stats.StrikeOuts)
	}
}

func TestGetHeadToHead(t *testing.T) {
	ts := fixtureServer(t, "../../testdata/season_schedule.json")
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		teamID:     RockiesID,
		now:        func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) },
	}

	h2h, err := c.getH2HFromURL(ts.URL, 117) // vs Astros
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h2h.OpponentName != "Houston Astros" {
		t.Errorf("opponent = %q, want %q", h2h.OpponentName, "Houston Astros")
	}
	if h2h.GamesPlayed != 2 {
		t.Errorf("games = %d, want 2", h2h.GamesPlayed)
	}
	if h2h.Wins != 1 || h2h.Losses != 1 {
		t.Errorf("h2h = %d-%d, want 1-1", h2h.Wins, h2h.Losses)
	}
}

func TestGetHeadToHead_NoGames(t *testing.T) {
	ts := fixtureServer(t, "../../testdata/season_schedule.json")
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		teamID:     RockiesID,
		now:        func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) },
	}

	h2h, err := c.getH2HFromURL(ts.URL, 999) // team we haven't played
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h2h.GamesPlayed != 0 {
		t.Errorf("games = %d, want 0", h2h.GamesPlayed)
	}
}

func TestGameHelpers(t *testing.T) {
	game := &Game{
		GameDateTime: time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC),
		IsHome:       true,
		HomeTeam:     TeamInfo{ID: 115, Name: "Colorado Rockies", Wins: 5, Losses: 6},
		AwayTeam:     TeamInfo{ID: 117, Name: "Houston Astros", Wins: 7, Losses: 4},
	}

	if game.Opponent().Name != "Houston Astros" {
		t.Errorf("opponent = %q, want Houston Astros", game.Opponent().Name)
	}
	if game.RockiesTeam().Name != "Colorado Rockies" {
		t.Errorf("rockies = %q, want Colorado Rockies", game.RockiesTeam().Name)
	}
	if game.OpponentID() != 117 {
		t.Errorf("opponent ID = %d, want 117", game.OpponentID())
	}
	if game.RockiesTeam().WinLossString() != "5-6" {
		t.Errorf("W-L = %q, want 5-6", game.RockiesTeam().WinLossString())
	}
}
