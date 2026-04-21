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

	accuracy := CalculateFactorAccuracy(predictions)

	if len(accuracy) == 0 {
		t.Error("expected accuracy map to have entries")
	}
}

func TestCalculateFactorAccuracy_EmptyPredictions(t *testing.T) {
	accuracy := CalculateFactorAccuracy(nil)
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
			Actual:   "", // no result yet
			Factors: FactorScores{WinRate: 0.6},
		},
	}
	accuracy := CalculateFactorAccuracy(predictions)
	if len(accuracy) == 0 {
		t.Error("expected map returned")
	}
}

func TestAdjustWeights(t *testing.T) {
	current := Weights{
		WinRate: 0.30,
		Pitcher: 0.30,
		H2H:     0.15,
		HomeAway: 0.10,
		Momentum: 0.05,
		Stars:    0.10,
	}

	accuracy := map[string]float64{
		"winRate":   0.60, // good
		"pitcher":  0.40, // bad
		"h2h":      0.50, // neutral
	}

	newWeights := AdjustWeights(current, accuracy, 10)

	if newWeights.WinRate <= current.WinRate {
		t.Errorf("good factor should increase: winRate %f -> %f", current.WinRate, newWeights.WinRate)
	}
	if newWeights.Pitcher >= current.Pitcher {
		t.Errorf("bad factor should decrease: pitcher %f -> %f", current.Pitcher, newWeights.Pitcher)
	}
}

func TestAdjustWeights_SmallSample(t *testing.T) {
	current := Weights{WinRate: 0.30, Pitcher: 0.30}

	accuracy := map[string]float64{
		"winRate": 0.70, // good
		"pitcher": 0.40, // bad
	}

	newWeights := AdjustWeights(current, accuracy, 2)
	if newWeights.WinRate == current.WinRate {
		t.Errorf("should adjust with < 3 predictions, got %f", newWeights.WinRate)
	}
}

func TestAdjustWeights_Bounds(t *testing.T) {
	current := Weights{WinRate: 0.40, Pitcher: 0.10}

	accuracy := map[string]float64{
		"winRate": 0.70, // good - should increase
		"pitcher": 0.40, // bad - should decrease
	}

	newWeights := AdjustWeights(current, accuracy, 10)
	// With both good and bad factors, should rebalance
	if newWeights.WinRate == current.WinRate && newWeights.Pitcher == current.Pitcher {
		t.Error("weights should rebalance with both good and bad factors")
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