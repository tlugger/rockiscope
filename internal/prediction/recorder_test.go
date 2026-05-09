package prediction

import (
	"testing"
)

func TestCalculateFactorAccuracy(t *testing.T) {
	predictions := []PredictionRecord{
		{
			Predicted: "W",
			Actual:   "W",
			Factors: FactorScores{WinRate: 0.6, Pitcher: 0.3, H2H: 0.2, HomeAway: 0.1},
		},
		{
			Predicted: "W",
			Actual:   "W",
			Factors: FactorScores{WinRate: 0.7, Pitcher: 0.4, H2H: 0.3, HomeAway: 0.2},
		},
		{
			Predicted: "L",
			Actual:   "L",
			Factors: FactorScores{WinRate: 0.3, Pitcher: 0.2, H2H: 0.1, HomeAway: 0.1},
		},
		{
			Predicted: "W",
			Actual:   "L",
			Factors: FactorScores{WinRate: 0.6, Pitcher: 0.5, H2H: 0.4, HomeAway: 0.3},
		},
	}

	accuracy, samples := CalculateFactorAccuracy(predictions)

	if len(accuracy) == 0 {
		t.Error("expected accuracy map to have entries")
	}
	if samples["winRate"] != 4 {
		t.Errorf("expected 4 winRate samples, got %d", samples["winRate"])
	}
}

func TestCalculateFactorAccuracy_EmptyPredictions(t *testing.T) {
	accuracy, _ := CalculateFactorAccuracy(nil)
	if accuracy == nil {
		t.Error("expected map returned")
	}
	if accuracy["winRate"] != 0.5 {
		t.Errorf("expected default 0.5, got %f", accuracy["winRate"])
	}
}

func TestCalculateFactorAccuracy_NoActuals(t *testing.T) {
	predictions := []PredictionRecord{
		{
			Predicted: "W",
			Actual:   "",
			Factors: FactorScores{WinRate: 0.6},
		},
	}
	accuracy, _ := CalculateFactorAccuracy(predictions)
	if len(accuracy) == 0 {
		t.Error("expected map returned")
	}
}

func TestAdjustWeights(t *testing.T) {
	current := DefaultWeights()

	accuracy := map[string]float64{
		"winRate":  0.70,
		"pitcher":  0.30,
		"h2h":     0.50,
		"homeAway": 0.50,
		"momentum": 0.50,
		"stars":    0.50,
	}
	samples := map[string]int{
		"winRate": 10, "pitcher": 10, "h2h": 10,
		"homeAway": 10, "momentum": 10, "stars": 10,
	}

	newWeights := AdjustWeights(current, accuracy, samples, 25)

	if newWeights.WinRate <= current.WinRate {
		t.Errorf("accurate factor should increase: winRate %f -> %f", current.WinRate, newWeights.WinRate)
	}
	if newWeights.Pitcher >= current.Pitcher {
		t.Errorf("inaccurate factor should decrease: pitcher %f -> %f", current.Pitcher, newWeights.Pitcher)
	}
}

func TestAdjustWeights_TooFewGames(t *testing.T) {
	current := DefaultWeights()

	accuracy := map[string]float64{"winRate": 0.90, "pitcher": 0.10}
	samples := map[string]int{"winRate": 3, "pitcher": 3}

	newWeights := AdjustWeights(current, accuracy, samples, 3)
	if newWeights.WinRate != current.WinRate {
		t.Errorf("should not adjust with < 5 total games: %f -> %f", current.WinRate, newWeights.WinRate)
	}
}

func TestAdjustWeights_MinSamplesPerFactor(t *testing.T) {
	current := DefaultWeights()

	accuracy := map[string]float64{
		"winRate": 0.80, "pitcher": 0.80,
		"h2h": 0.80, "homeAway": 0.50,
		"momentum": 0.50, "stars": 0.50,
	}
	samples := map[string]int{
		"winRate": 10, "pitcher": 10,
		"h2h": 2, "homeAway": 10,
		"momentum": 10, "stars": 10,
	}

	newWeights := AdjustWeights(current, accuracy, samples, 10)
	// H2H has too few samples — should regularize toward default, not boost
	if newWeights.H2H > current.H2H+0.01 {
		t.Errorf("factor with < 5 samples should not get boosted: h2h %f -> %f", current.H2H, newWeights.H2H)
	}
}

func TestAdjustWeights_DecayingLearningRate(t *testing.T) {
	current := DefaultWeights()

	accuracy := map[string]float64{
		"winRate": 0.80, "pitcher": 0.50,
		"h2h": 0.50, "homeAway": 0.50,
		"momentum": 0.50, "stars": 0.50,
	}
	samples := map[string]int{
		"winRate": 20, "pitcher": 20,
		"h2h": 20, "homeAway": 20,
		"momentum": 20, "stars": 20,
	}

	early := AdjustWeights(current, accuracy, samples, 10)
	late := AdjustWeights(current, accuracy, samples, 100)

	earlyShift := early.WinRate - current.WinRate
	lateShift := late.WinRate - current.WinRate
	if lateShift >= earlyShift {
		t.Errorf("later adjustments should be smaller: early=%f late=%f", earlyShift, lateShift)
	}
}

func TestPredictionHistory_Add(t *testing.T) {
	h := &PredictionHistory{
		Predictions: []PredictionRecord{},
		Current:     DefaultWeights(),
	}

	h.Add(PredictionRecord{Date: "2026-04-08", Predicted: "W"})

	if len(h.Predictions) != 1 {
		t.Errorf("expected 1 prediction, got %d", len(h.Predictions))
	}
	if h.Predictions[0].Date != "2026-04-08" {
		t.Errorf("date mismatch: %s", h.Predictions[0].Date)
	}
}

func TestPredictionHistory_AddLimit(t *testing.T) {
	h := &PredictionHistory{
		Predictions: make([]PredictionRecord, 50),
		Current:    DefaultWeights(),
	}

	for i := range h.Predictions {
		h.Predictions[i] = PredictionRecord{Date: "2026-04-01"}
	}

	h.Add(PredictionRecord{Date: "2026-04-50"})

	if len(h.Predictions) > 50 {
		t.Errorf("should cap at 50: %d", len(h.Predictions))
	}
}

func TestPredictionHistory_Recent(t *testing.T) {
	h := &PredictionHistory{
		Predictions: []PredictionRecord{
			{Date: "2026-04-01"},
			{Date: "2026-04-02"},
			{Date: "2026-04-03"},
			{Date: "2026-04-04"},
		},
		Current: DefaultWeights(),
	}

	recent := h.Recent(2)
	if len(recent) != 2 {
		t.Errorf("expected 2, got %d", len(recent))
	}
	if recent[0].Date != "2026-04-03" {
		t.Errorf("should be most recent first")
	}
}

func TestPredictionHistory_Recent_TooMany(t *testing.T) {
	h := &PredictionHistory{
		Predictions: []PredictionRecord{
			{Date: "2026-04-01"},
		},
		Current: DefaultWeights(),
	}

	recent := h.Recent(10)
	if len(recent) != 1 {
		t.Errorf("expected 1, got %d", len(recent))
	}
}

func TestPredictionHistory_CorrectCount(t *testing.T) {
	h := &PredictionHistory{
		Predictions: []PredictionRecord{
			{Predicted: "W", Actual: "W"},
			{Predicted: "W", Actual: "L"},
			{Predicted: "L", Actual: "L"},
			{Predicted: "L", Actual: ""}, // no result
		},
		Current: DefaultWeights(),
	}

	if h.CorrectCount() != 2 {
		t.Errorf("expected 2 correct, got %d", h.CorrectCount())
	}
}

func TestWeightsTotal(t *testing.T) {
	w := Weights{
		WinRate: 0.30,
		Pitcher: 0.30,
		H2H:     0.15,
		HomeAway: 0.10,
		Momentum: 0.05,
		Stars:    0.10,
	}

	total := w.total()
	if total != 1.0 {
		t.Errorf("expected 1.0, got %f", total)
	}
}