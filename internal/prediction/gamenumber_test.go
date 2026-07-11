package prediction

import (
	"encoding/json"
	"strings"
	"testing"
)

// GameNumber must round-trip through JSON and be omitted when zero (single games),
// so double-header records stay distinguishable without bloating every entry.
func TestPredictionRecord_GameNumberJSON(t *testing.T) {
	dh := PredictionRecord{
		Date: "2026-04-26", Opponent: "New York Mets", Predicted: "W",
		GamePK: 823637, GameNumber: 2, WinProbability: 0.53, Confidence: 53,
	}
	data, err := json.Marshal(dh)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"gameNumber":2`) {
		t.Errorf("expected gameNumber in JSON, got %s", data)
	}

	var back PredictionRecord
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.GameNumber != 2 {
		t.Errorf("GameNumber round-trip = %d, want 2", back.GameNumber)
	}

	single := PredictionRecord{Date: "2026-04-24", Opponent: "New York Mets", GamePK: 823638}
	data, _ = json.Marshal(single)
	if strings.Contains(string(data), "gameNumber") {
		t.Errorf("gameNumber should be omitted when zero, got %s", data)
	}
}
