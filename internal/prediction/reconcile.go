package prediction

import (
	"sort"
	"strconv"

	"github.com/tlugger/rockiscope/internal/mlb"
)

// ReconcileResult counts what Reconcile changed.
type ReconcileResult struct {
	Created       int // synthetic records added for games with no record
	ScoresFilled  int
	ActualsFilled int
}

func (r ReconcileResult) Changed() bool {
	return r.Created+r.ScoresFilled+r.ActualsFilled > 0
}

// Reconcile brings the history in line with MLB's completed games: every final
// game without a record gets a synthetic one (counted in the W-L, never in
// prediction accuracy), and missing scores and results are filled in.
//
// A posted prediction dated on or after protectFrom is never marked settled
// here, because that's the follow-up reply's job, and the reply only fires for
// unsettled records. Pass "" to settle everything.
func Reconcile(h *PredictionHistory, results []mlb.GameResult, protectFrom string) ReconcileResult {
	var res ReconcileResult
	// Indices, not pointers: appending synthetic records can reallocate the slice.
	byGamePk := map[int]int{}
	byDateOpp := map[string]int{}
	for i, p := range h.Predictions {
		if p.GamePK != 0 {
			byGamePk[p.GamePK] = i
		}
		byDateOpp[p.Date+"|"+p.Opponent] = i
	}

	for _, gr := range results {
		actual := "L"
		if gr.Won {
			actual = "W"
		}
		i, ok := byGamePk[gr.GamePk]
		if !ok && gr.GamePk == 0 {
			i, ok = byDateOpp[gr.Date+"|"+gr.Opponent]
		}

		if !ok {
			h.Predictions = append(h.Predictions, PredictionRecord{
				Date:           gr.Date,
				Opponent:       gr.Opponent,
				IsHome:         gr.IsHome,
				Predicted:      "L",
				Confidence:     50,
				Actual:         actual,
				RockiesScore:   gr.RockiesScore,
				OppScore:       gr.OppScore,
				GamePK:         gr.GamePk,
				GameNumber:     gr.GameNumber,
				WinProbability: 0.5,
				Synthetic:      true,
			})
			if gr.GamePk != 0 {
				byGamePk[gr.GamePk] = len(h.Predictions) - 1
			}
			res.Created++
			continue
		}

		existing := &h.Predictions[i]
		if existing.GamePK == 0 {
			existing.GamePK = gr.GamePk
		}
		awaitingReply := protectFrom != "" && existing.PostURI != "" && existing.Actual == "" && existing.Date >= protectFrom
		if awaitingReply {
			continue
		}
		if existing.Actual == "" {
			existing.Actual = actual
			res.ActualsFilled++
		}
		if existing.RockiesScore == 0 && existing.OppScore == 0 && (gr.RockiesScore != 0 || gr.OppScore != 0) {
			existing.RockiesScore = gr.RockiesScore
			existing.OppScore = gr.OppScore
			res.ScoresFilled++
		}
	}

	if res.Created > 0 {
		h.SortByDate()
	}
	return res
}

func recordKey(p PredictionRecord) string {
	if p.GamePK != 0 {
		return "pk:" + strconv.Itoa(p.GamePK)
	}
	return p.Date + "|" + p.Opponent
}

// Merge folds another copy of the history (usually the one on disk, written by
// a `backfill` run) into h. Records missing from h are added; for the same
// game, a real prediction beats a synthetic one and blank fields are filled.
// h's weights win: the running bot owns the model. It returns how many
// records were added or improved.
func (h *PredictionHistory) Merge(other *PredictionHistory) int {
	if other == nil {
		return 0
	}
	index := map[string]int{}
	for i, p := range h.Predictions {
		index[recordKey(p)] = i
	}

	changed := 0
	for _, o := range other.Predictions {
		i, ok := index[recordKey(o)]
		if !ok {
			index[recordKey(o)] = len(h.Predictions)
			h.Predictions = append(h.Predictions, o)
			changed++
			continue
		}
		p := &h.Predictions[i]
		before := *p
		if p.Synthetic && !o.Synthetic {
			merged := o
			if merged.Actual == "" {
				merged.Actual = p.Actual
			}
			if merged.RockiesScore == 0 && merged.OppScore == 0 {
				merged.RockiesScore, merged.OppScore = p.RockiesScore, p.OppScore
			}
			*p = merged
		} else {
			if p.Actual == "" {
				p.Actual = o.Actual
			}
			if p.RockiesScore == 0 && p.OppScore == 0 {
				p.RockiesScore, p.OppScore = o.RockiesScore, o.OppScore
			}
			if p.PostURI == "" {
				p.PostURI = o.PostURI
			}
			if p.GamePK == 0 {
				p.GamePK = o.GamePK
			}
			if p.GameNumber == 0 {
				p.GameNumber = o.GameNumber
			}
			if p.Factors == (FactorScores{}) {
				p.Factors = o.Factors
			}
		}
		if *p != before {
			changed++
		}
	}
	if changed > 0 {
		h.SortByDate()
	}
	return changed
}

// SortByDate orders records by date, then game number (double-headers).
func (h *PredictionHistory) SortByDate() {
	sort.SliceStable(h.Predictions, func(i, j int) bool {
		a, b := h.Predictions[i], h.Predictions[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return a.GameNumber < b.GameNumber
	})
}
