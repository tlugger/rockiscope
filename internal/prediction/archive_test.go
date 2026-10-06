package prediction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func seasonRecords() []PredictionRecord {
	return []PredictionRecord{
		{Date: "2026-04-01", Opponent: "A", Predicted: "L", Actual: "L", WinProbability: 0.30, Factors: FactorScores{Stars: 0.2}},
		{Date: "2026-04-02", Opponent: "B", Predicted: "L", Actual: "L", WinProbability: 0.20, Factors: FactorScores{Stars: 0.9}},
		{Date: "2026-04-03", Opponent: "C", Predicted: "W", Actual: "L", WinProbability: 0.70},
		{Date: "2026-04-04", Opponent: "D", Predicted: "L", Actual: "W", WinProbability: 0.45},
		{Date: "2026-04-05", Opponent: "Off Day", Predicted: "N/A"},
		{Date: "2026-03-30", Opponent: "E", Predicted: "L", Actual: "W", Synthetic: true},
	}
}

func TestSummarizeSeason(t *testing.T) {
	s := SummarizeSeason(seasonRecords(), 2026, DefaultWeights())

	if s.RockiesWins != 2 || s.RockiesLosses != 3 {
		t.Errorf("record = %d-%d, want 2-3 (synthetic counts toward W-L)", s.RockiesWins, s.RockiesLosses)
	}
	if s.Predictions != 4 || s.Correct != 2 {
		t.Errorf("predictions %d/%d, want 2/4", s.Correct, s.Predictions)
	}
	if s.Accuracy != 0.5 {
		t.Errorf("accuracy = %v", s.Accuracy)
	}
	if s.PickedLosses != 3 || s.PickedWins != 1 {
		t.Errorf("picked %dW %dL", s.PickedWins, s.PickedLosses)
	}
	if s.LongestCorrectStreak != 2 {
		t.Errorf("streak = %d", s.LongestCorrectStreak)
	}
	if s.BestCall == nil || s.BestCall.Opponent != "B" {
		t.Errorf("best call = %+v", s.BestCall)
	}
	if s.WorstMiss == nil || s.WorstMiss.Opponent != "C" {
		t.Errorf("worst miss = %+v", s.WorstMiss)
	}
}

func TestSummarizeSeason_IgnoresOtherSeasons(t *testing.T) {
	recs := append(seasonRecords(), PredictionRecord{Date: "2025-09-01", Predicted: "W", Actual: "W"})
	s := SummarizeSeason(recs, 2026, DefaultWeights())
	if s.Predictions != 4 {
		t.Errorf("predictions = %d", s.Predictions)
	}
}

func TestArchiveAndReset(t *testing.T) {
	dir := t.TempDir()
	learned := Weights{WinRate: 0.2, Pitcher: 0.4, H2H: 0.05, HomeAway: 0.15, Momentum: 0.1, Stars: 0.1}
	h := &PredictionHistory{Predictions: seasonRecords(), Current: learned}
	if err := SaveHistory(h, dir); err != nil {
		t.Fatal(err)
	}

	sum, err := ArchiveAndReset(h, dir, 2027, time.Date(2027, 2, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if sum == nil || sum.Season != 2026 || sum.FinalWeights != learned {
		t.Fatalf("summary = %+v", sum)
	}
	if len(h.Predictions) != 0 || h.Current != DefaultWeights() {
		t.Errorf("live history not reset: %d records, weights %+v", len(h.Predictions), h.Current)
	}

	var archived PredictionHistory
	data, err := os.ReadFile(filepath.Join(dir, "archive", "2026", "prediction_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &archived)
	if len(archived.Predictions) != 6 || archived.Current != learned {
		t.Errorf("archived %d records, weights %+v", len(archived.Predictions), archived.Current)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "2026", "summary.json")); err != nil {
		t.Errorf("summary missing: %v", err)
	}

	reloaded, _ := LoadHistory(dir)
	if len(reloaded.Predictions) != 0 {
		t.Errorf("reloaded history has %d records", len(reloaded.Predictions))
	}

	// Second call is a no-op.
	again, err := ArchiveAndReset(h, dir, 2027, time.Now())
	if err != nil || again != nil {
		t.Errorf("expected no-op, got %+v, %v", again, err)
	}
}

func TestArchiveAndReset_KeepsCurrentSeason(t *testing.T) {
	h := &PredictionHistory{
		Predictions: []PredictionRecord{{Date: "2026-09-01"}, {Date: "2027-03-01"}},
		Current:     DefaultWeights(),
	}
	if _, err := ArchiveAndReset(h, "", 2027, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(h.Predictions) != 1 || h.Predictions[0].Date != "2027-03-01" {
		t.Errorf("kept %+v", h.Predictions)
	}
}

func TestLoadHistory_RecoversFromBackup(t *testing.T) {
	dir := t.TempDir()
	h := &PredictionHistory{Predictions: seasonRecords(), Current: DefaultWeights()}
	SaveHistory(h, dir)
	SaveHistory(h, dir) // second save creates the .bak
	os.WriteFile(filepath.Join(dir, historyFileName), []byte(`{"predictions": [`), 0644)

	got, err := LoadHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Predictions) != 6 {
		t.Errorf("recovered %d records", len(got.Predictions))
	}
}
