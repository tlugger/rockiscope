package scheduler

import (
	"testing"

	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// homeGame builds a Rockies home game vs the given opponent.
func homeGame(pk, gameNumber int, officialDate, opponent string) *mlb.Game {
	return &mlb.Game{
		GamePk:       pk,
		OfficialDate: officialDate,
		GameNumber:   gameNumber,
		IsHome:       true,
		HomeTeam:     mlb.TeamInfo{ID: mlb.RockiesID, Name: "Colorado Rockies"},
		AwayTeam:     mlb.TeamInfo{ID: 121, Name: opponent},
	}
}

// recordPrediction must stamp the record with MLB's official date (not the wall
// clock) and carry the double-header game number.
func TestRecordPrediction_UsesOfficialDateAndGameNumber(t *testing.T) {
	s := &Scheduler{
		logger:      testLogger(),
		dataDir:     "",
		predHistory: &prediction.PredictionHistory{Current: prediction.Weights{WinRate: 1}},
	}

	game := homeGame(823637, 2, "2026-04-26", "New York Mets")
	pred := prediction.Prediction{WinProbability: 0.53, Pick: "W"}

	// Pass a WRONG wall-clock date; the record should use the official date.
	s.recordPrediction("2026-04-25", game, pred, "at://post/1")

	if len(s.predHistory.Predictions) != 1 {
		t.Fatalf("expected 1 record, got %d", len(s.predHistory.Predictions))
	}
	rec := s.predHistory.Predictions[0]
	if rec.Date != "2026-04-26" {
		t.Errorf("Date = %q, want official 2026-04-26", rec.Date)
	}
	if rec.GameNumber != 2 {
		t.Errorf("GameNumber = %d, want 2", rec.GameNumber)
	}
	if rec.GamePK != 823637 {
		t.Errorf("GamePK = %d, want 823637", rec.GamePK)
	}
}

// A double-header's two games share date+opponent but have distinct gamePks.
// Each result must settle its own record, not the first one it sees.
func TestProcessCompletedGame_DoubleHeaderSettlesByGamePk(t *testing.T) {
	poster := &mockPoster{}
	s := &Scheduler{
		poster:  poster,
		logger:  testLogger(),
		dataDir: "",
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{
				{Date: "2026-04-26", Opponent: "New York Mets", Predicted: "L", GamePK: 823635, GameNumber: 1, PostURI: "at://g1"},
				{Date: "2026-04-26", Opponent: "New York Mets", Predicted: "W", GamePK: 823637, GameNumber: 2, PostURI: "at://g2"},
			},
			Current: prediction.Weights{WinRate: 1},
		},
	}

	// Game 2 finalizes first; it must settle record #2, not #1.
	s.processCompletedGame(mlb.GameResult{GamePk: 823637, Date: "2026-04-26", Opponent: "New York Mets", RockiesScore: 3, OppScore: 0, Won: true}, "2026-04-26")
	s.processCompletedGame(mlb.GameResult{GamePk: 823635, Date: "2026-04-26", Opponent: "New York Mets", RockiesScore: 3, OppScore: 1, Won: true}, "2026-04-26")

	g1 := s.predHistory.Predictions[0]
	g2 := s.predHistory.Predictions[1]
	if g1.Actual != "W" || g1.RockiesScore != 3 || g1.OppScore != 1 {
		t.Errorf("game 1 settled wrong: actual=%s %d-%d", g1.Actual, g1.RockiesScore, g1.OppScore)
	}
	if g2.Actual != "W" || g2.RockiesScore != 3 || g2.OppScore != 0 {
		t.Errorf("game 2 settled wrong: actual=%s %d-%d", g2.Actual, g2.RockiesScore, g2.OppScore)
	}
}

// A prediction posted before a postponement should settle when the makeup plays,
// and the record's date should be corrected to the day it was actually played.
func TestProcessCompletedGame_PostponementRedatesRecord(t *testing.T) {
	poster := &mockPoster{}
	s := &Scheduler{
		poster:  poster,
		logger:  testLogger(),
		dataDir: "",
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{
				// Predicted for the originally scheduled 5/05 date.
				{Date: "2026-05-05", Opponent: "New York Mets", Predicted: "L", GamePK: 824362, PostURI: "at://p"},
			},
			Current: prediction.Weights{WinRate: 1},
		},
	}

	// Makeup played 5/07 (same gamePk, new official date), Rockies won.
	s.processCompletedGame(mlb.GameResult{GamePk: 824362, Date: "2026-05-07", Opponent: "New York Mets", RockiesScore: 6, OppScore: 2, Won: true}, "2026-05-07")

	rec := s.predHistory.Predictions[0]
	if rec.Actual != "W" {
		t.Errorf("actual = %q, want W", rec.Actual)
	}
	if rec.Date != "2026-05-07" {
		t.Errorf("Date = %q, want re-dated 2026-05-07", rec.Date)
	}
	if len(poster.replies) != 1 {
		t.Errorf("expected 1 follow-up reply, got %d", len(poster.replies))
	}
}
