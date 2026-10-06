package prediction

import (
	"testing"

	"github.com/tlugger/rockiscope/internal/mlb"
)

func results() []mlb.GameResult {
	return []mlb.GameResult{
		{GamePk: 1, Date: "2026-03-27", Opponent: "Miami Marlins", Won: false, RockiesScore: 0, OppScore: 4},
		{GamePk: 2, Date: "2026-03-28", Opponent: "Miami Marlins", Won: true, RockiesScore: 5, OppScore: 2},
		{GamePk: 3, Date: "2026-04-08", Opponent: "Houston Astros", Won: false, RockiesScore: 1, OppScore: 6},
		{GamePk: 4, Date: "2026-04-09", Opponent: "San Diego Padres", Won: true, RockiesScore: 3, OppScore: 0},
	}
}

func TestReconcile_AddsMissingGamesAsSynthetic(t *testing.T) {
	h := &PredictionHistory{Predictions: []PredictionRecord{
		{Date: "2026-04-08", Opponent: "Houston Astros", GamePK: 3, Predicted: "L", Actual: "L", PostURI: "at://p3"},
		{Date: "2026-04-09", Opponent: "San Diego Padres", GamePK: 4, Predicted: "W", PostURI: "at://p4"},
	}}
	res := Reconcile(h, results(), "")
	if res.Created != 2 || res.ActualsFilled != 1 || !res.Changed() {
		t.Fatalf("result = %+v", res)
	}
	if h.Predictions[0].GamePK != 1 || !h.Predictions[0].Synthetic || h.Predictions[0].Actual != "L" {
		t.Errorf("records not sorted/synthetic: %+v", h.Predictions[0])
	}
	sum := SummarizeSeason(h.Predictions, 2026, DefaultWeights())
	if sum.RockiesWins != 2 || sum.RockiesLosses != 2 {
		t.Errorf("W-L = %d-%d", sum.RockiesWins, sum.RockiesLosses)
	}
	if again := Reconcile(h, results(), ""); again.Changed() {
		t.Errorf("second run changed things: %+v", again)
	}
}

func TestReconcile_LeavesRecentPostedPredictionsForTheReply(t *testing.T) {
	h := &PredictionHistory{Predictions: []PredictionRecord{
		{Date: "2026-04-09", Opponent: "San Diego Padres", GamePK: 4, Predicted: "W", PostURI: "at://p4"},
	}}
	Reconcile(h, results(), "2026-04-08")
	for _, p := range h.Predictions {
		if p.GamePK == 4 && p.Actual != "" {
			t.Error("settled a prediction whose follow-up reply hasn't posted")
		}
	}
}

func TestMerge_RestoresBackfillTheBotNeverSaw(t *testing.T) {
	// The running bot's in-memory copy: no early-season games.
	mem := &PredictionHistory{Current: Weights{Stars: 0.07}, Predictions: []PredictionRecord{
		{Date: "2026-04-08", Opponent: "Houston Astros", GamePK: 3, Predicted: "L", Actual: "L", PostURI: "at://p3"},
		{Date: "2026-10-06", Opponent: "Off Day", Predicted: "N/A"},
	}}
	// What `backfill` wrote to disk meanwhile.
	disk := &PredictionHistory{Current: DefaultWeights(), Predictions: []PredictionRecord{
		{Date: "2026-03-27", Opponent: "Miami Marlins", GamePK: 1, Predicted: "L", Actual: "L", Synthetic: true},
		{Date: "2026-04-08", Opponent: "Houston Astros", GamePK: 3, Predicted: "L", Actual: "L", RockiesScore: 1, OppScore: 6},
	}}

	if n := mem.Merge(disk); n != 2 {
		t.Errorf("changed = %d, want 2 (one added, one score filled)", n)
	}
	if len(mem.Predictions) != 3 || mem.Predictions[0].GamePK != 1 {
		t.Fatalf("merged = %+v", mem.Predictions)
	}
	if p := mem.Predictions[1]; p.OppScore != 6 || p.PostURI != "at://p3" {
		t.Errorf("fields not combined: %+v", p)
	}
	if mem.Current.Stars != 0.07 {
		t.Error("the running bot's weights must win")
	}
	if mem.Merge(disk) != 0 || mem.Merge(nil) != 0 {
		t.Error("merge should be idempotent")
	}
}

func TestMerge_RealPredictionBeatsSynthetic(t *testing.T) {
	mem := &PredictionHistory{Predictions: []PredictionRecord{
		{Date: "2026-03-27", Opponent: "Miami Marlins", GamePK: 1, Predicted: "L", Actual: "L", Synthetic: true, RockiesScore: 0, OppScore: 4},
	}}
	disk := &PredictionHistory{Predictions: []PredictionRecord{
		{Date: "2026-03-27", Opponent: "Miami Marlins", GamePK: 1, Predicted: "W", PostURI: "at://p1", WinProbability: 0.55},
	}}
	mem.Merge(disk)
	p := mem.Predictions[0]
	if p.Synthetic || p.Predicted != "W" || p.Actual != "L" || p.OppScore != 4 {
		t.Errorf("merged = %+v", p)
	}
}

func TestReconcile_BackfillsStarterNamesOnly(t *testing.T) {
	withStats := &Starter{ID: 1, Name: "Kyle Freeland", ERA: 3.21, WHIP: 1.3, HasStats: true}
	h := &PredictionHistory{Predictions: []PredictionRecord{
		{Date: "2026-04-08", Opponent: "Houston Astros", GamePK: 3, Predicted: "L", Actual: "L", RockiesStarter: withStats},
	}}
	rs := []mlb.GameResult{
		{GamePk: 3, Date: "2026-04-08", Opponent: "Houston Astros", RockiesStarter: &mlb.PitcherInfo{ID: 9, FullName: "Someone Else"}, OppStarter: &mlb.PitcherInfo{ID: 2, FullName: "Framber Valdez"}},
		{GamePk: 1, Date: "2026-03-27", Opponent: "Miami Marlins", Won: false, RockiesStarter: &mlb.PitcherInfo{ID: 1, FullName: "Kyle Freeland"}},
	}
	res := Reconcile(h, rs, "")
	if res.StartersFilled != 1 || res.Created != 1 {
		t.Fatalf("result = %+v", res)
	}
	byPk := map[int]PredictionRecord{}
	for _, p := range h.Predictions {
		byPk[p.GamePK] = p
	}
	if p := byPk[3]; p.RockiesStarter != withStats || p.OppStarter == nil || p.OppStarter.Name != "Framber Valdez" || p.OppStarter.HasStats {
		t.Errorf("existing record: %+v / %+v", p.RockiesStarter, p.OppStarter)
	}
	if p := byPk[1]; p.RockiesStarter == nil || p.RockiesStarter.Name != "Kyle Freeland" || p.RockiesStarter.HasStats {
		t.Errorf("synthetic record starter: %+v", p.RockiesStarter)
	}
}

func TestNewStarter(t *testing.T) {
	if NewStarter(nil, nil) != nil {
		t.Error("no pitcher should mean no starter")
	}
	s := NewStarter(&mlb.PitcherInfo{ID: 5, FullName: "A"}, &mlb.PitcherStats{ERA: 4.5, WHIP: 1.2, InningsPitched: 30})
	if !s.HasStats || s.ERA != 4.5 {
		t.Errorf("starter = %+v", s)
	}
	if noIP := NewStarter(&mlb.PitcherInfo{ID: 5, FullName: "A"}, &mlb.PitcherStats{}); noIP.HasStats {
		t.Error("zero innings shouldn't count as stats")
	}
}
