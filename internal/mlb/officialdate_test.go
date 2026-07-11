package mlb

import "testing"

// makeScheduleGame builds a minimal scheduleGame with the Rockies away vs opp.
func rockiesAwayGame(pk int, gameDate, officialDate string, awayScore, homeScore int, abstract, detailed string) scheduleGame {
	var g scheduleGame
	g.GamePk = pk
	g.GameDate = gameDate
	g.OfficialDate = officialDate
	g.Status.AbstractGameState = abstract
	g.Status.DetailedState = detailed
	g.Teams.Away.Team.ID = RockiesID
	g.Teams.Away.Team.Name = "Colorado Rockies"
	g.Teams.Away.Score = awayScore
	g.Teams.Home.Team.ID = 135
	g.Teams.Home.Team.Name = "San Diego Padres"
	g.Teams.Home.Score = homeScore
	return g
}

func TestParseGame_OfficialDateRescheduleAndGameNumber(t *testing.T) {
	c := &Client{teamID: RockiesID}

	g := rockiesAwayGame(823319, "2026-04-10T01:40:00Z", "2026-04-09", 3, 7, "Final", "Final")
	g.GameNumber = 2
	g.DoubleHeader = "S"
	g.RescheduleGameDate = "2026-04-09"

	game, err := c.parseGame(g)
	if err != nil {
		t.Fatalf("parseGame: %v", err)
	}
	// The UTC gameDate is 2026-04-10, but the official (local) date is 2026-04-09.
	if game.OfficialDate != "2026-04-09" {
		t.Errorf("OfficialDate = %q, want 2026-04-09", game.OfficialDate)
	}
	if game.GameNumber != 2 {
		t.Errorf("GameNumber = %d, want 2", game.GameNumber)
	}
	if game.DoubleHeader != "S" {
		t.Errorf("DoubleHeader = %q, want S", game.DoubleHeader)
	}
	if game.RescheduleGameDate != "2026-04-09" {
		t.Errorf("RescheduleGameDate = %q, want 2026-04-09", game.RescheduleGameDate)
	}
	if game.IsHome {
		t.Error("IsHome = true, want false (Rockies away)")
	}
}

func TestParseGame_OfficialDateFallsBackToGameDate(t *testing.T) {
	c := &Client{teamID: RockiesID}
	g := rockiesAwayGame(1, "2026-05-01T18:10:00Z", "", 0, 0, "Preview", "Scheduled")

	game, err := c.parseGame(g)
	if err != nil {
		t.Fatalf("parseGame: %v", err)
	}
	if game.OfficialDate != "2026-05-01" {
		t.Errorf("OfficialDate = %q, want fallback 2026-05-01", game.OfficialDate)
	}
}

func TestParseGameResults_UsesOfficialDateAndSkipsPostponed(t *testing.T) {
	c := &Client{teamID: RockiesID}

	var resp scheduleResponse
	resp.Dates = []struct {
		Games []scheduleGame `json:"games"`
	}{
		{
			Games: []scheduleGame{
				// Real Final game: UTC date rolls to 04-10, official date is 04-09.
				rockiesAwayGame(823319, "2026-04-10T01:40:00Z", "2026-04-09", 3, 7, "Final", "Final"),
				// Postponed slot: abstractGameState "Final" but detailedState "Postponed",
				// carries a bogus 0-0 score. Must be skipped.
				rockiesAwayGame(824362, "2026-05-06T00:40:00Z", "2026-05-07", 0, 0, "Final", "Postponed"),
			},
		},
	}

	results := c.parseGameResults(resp)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (postponed skipped), got %d", len(results))
	}
	gr := results[0]
	if gr.GamePk != 823319 {
		t.Errorf("GamePk = %d, want 823319", gr.GamePk)
	}
	if gr.Date != "2026-04-09" {
		t.Errorf("Date = %q, want official date 2026-04-09", gr.Date)
	}
	if gr.GameDateTime.IsZero() {
		t.Error("GameDateTime should be populated")
	}
	if gr.Opponent != "San Diego Padres" || gr.IsHome {
		t.Errorf("opponent/home wrong: %q home=%v", gr.Opponent, gr.IsHome)
	}
	if gr.RockiesScore != 3 || gr.OppScore != 7 || gr.Won {
		t.Errorf("score/won wrong: %d-%d won=%v", gr.RockiesScore, gr.OppScore, gr.Won)
	}
}
