package mlb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// fixtureClient serves recorded Stats API responses keyed by URL path prefix.
func fixtureClient(t *testing.T, routes map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for prefix, file := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				data, err := os.ReadFile("testdata/" + file)
				if err != nil {
					t.Fatalf("fixture %s: %v", file, err)
				}
				w.Write(data)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.Client(), nil)
	c.apiBase = srv.URL
	c.minInterval = 0
	c.sleep = func(time.Duration) {}
	return c
}

func TestGetSeasonDates(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/seasons/": "seasons_2027.json"})
	d, err := c.GetSeasonDates(2027)
	if err != nil {
		t.Fatal(err)
	}
	if d.SpringStart != "2027-02-19" || d.RegularStart != "2027-03-25" || d.PostEnd != "2027-10-31" {
		t.Errorf("dates = %+v", d)
	}
}

func TestGetPostseasonGames(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/schedule": "postseason_2026.json"})
	games, err := c.GetPostseasonGames(2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) < 20 {
		t.Fatalf("only %d games", len(games))
	}

	var first PostseasonGame
	for _, g := range games {
		if g.GamePk == 849845 {
			first = g
		}
	}
	if first.GameType != "F" || first.Status != "Final" || first.GamesInSeries != 3 {
		t.Errorf("wild card game = %+v", first)
	}
	if !first.Home.IsWinner || first.Away.IsWinner || first.Home.Score != 5 {
		t.Errorf("winner flags wrong: %+v", first)
	}
	if first.SeriesResult == "" || first.SeriesShort == "" || first.Venue == "" {
		t.Errorf("missing series/venue context: %+v", first)
	}
	team, opp, home := first.Side(144)
	if team.ID != 144 || opp.ID != 143 || !home || !first.Involves(143) {
		t.Errorf("Side() wrong")
	}
}

func TestGetRosterAndPeople(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/teams/": "roster_135.json", "/people": "people.json"})
	roster, err := c.GetRoster(135)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 26 || roster[0].BirthDate == "" {
		t.Errorf("roster = %d players, first %+v", len(roster), roster[0])
	}

	people, err := c.GetPeople([]int{608566, 547179})
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 2 || people[0].BirthDate != "1995-02-22" || people[0].Position != "P" {
		t.Errorf("people = %+v", people)
	}
	if none, err := c.GetPeople(nil); none != nil || err != nil {
		t.Error("empty lookup should be a no-op")
	}
}

func TestGetTransactions(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/transactions": "transactions.json"})
	txns, err := c.GetTransactions("2025-11-01", "2025-12-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) == 0 {
		t.Fatal("no transactions")
	}
	codes := map[string]bool{}
	for _, tx := range txns {
		codes[tx.TypeCode] = true
		if tx.ID == 0 || tx.Date == "" || tx.Description == "" {
			t.Errorf("incomplete txn %+v", tx)
		}
		if strings.Contains(tx.Description, "  ") {
			t.Errorf("whitespace not normalized: %q", tx.Description)
		}
	}
	if !codes["SFA"] || !codes["DFA"] {
		t.Errorf("expected SFA and DFA codes, got %v", codes)
	}
}

func TestGetRockiesGamesBetween(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/schedule": "opening_2027.json"})
	games, err := c.GetRockiesGamesBetween("2027-03-25", "2027-04-08", "R")
	if err != nil {
		t.Fatal(err)
	}
	if len(games) == 0 || games[0].OfficialDate != "2027-03-25" || games[0].GameType != "R" {
		t.Fatalf("games = %+v", games)
	}
	if games[0].Opponent().Name != "San Francisco Giants" {
		t.Errorf("opponent = %s", games[0].Opponent().Name)
	}
}

func TestSeasonEndpoints_PropagateErrors(t *testing.T) {
	c := fixtureClient(t, map[string]string{})
	if _, err := c.GetSeasonDates(2027); err == nil {
		t.Error("expected error")
	}
	if _, err := c.GetPostseasonGames(2026); err == nil {
		t.Error("expected error")
	}
	if _, err := c.GetTransactions("a", "b"); err == nil {
		t.Error("expected error")
	}
	if _, err := c.GetTeamRecordFor(115, 2026); err == nil {
		t.Error("expected error")
	}
}

func TestGetSeasonResultsFor_IncludesStarters(t *testing.T) {
	c := fixtureClient(t, map[string]string{"/schedule": "results_starters.json"})
	results, err := c.GetSeasonResultsFor(2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	opener := results[0]
	if opener.GamePk != 823893 || opener.IsHome {
		t.Fatalf("opener = %+v", opener)
	}
	if opener.RockiesStarter == nil || opener.RockiesStarter.FullName != "Kyle Freeland" {
		t.Errorf("Rockies starter = %+v", opener.RockiesStarter)
	}
	if opener.OppStarter == nil || opener.OppStarter.FullName != "Sandy Alcantara" {
		t.Errorf("opponent starter = %+v", opener.OppStarter)
	}
}
