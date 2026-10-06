package prediction

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tlugger/rockiscope/internal/fsutil"
)

// ArchiveDirName is the data-dir subfolder holding one folder per finished season.
const ArchiveDirName = "archive"

// SeasonSummary is the end-of-season scorecard. It powers the report card post
// and is written alongside the archived history at rollover.
type SeasonSummary struct {
	Season               int                `json:"season"`
	ArchivedAt           string             `json:"archivedAt,omitempty"`
	RockiesWins          int                `json:"rockiesWins"`
	RockiesLosses        int                `json:"rockiesLosses"`
	Predictions          int                `json:"predictions"`
	Correct              int                `json:"correct"`
	Accuracy             float64            `json:"accuracy"`
	PickedWins           int                `json:"pickedWins"`
	PickedLosses         int                `json:"pickedLosses"`
	LongestCorrectStreak int                `json:"longestCorrectStreak"`
	FactorAccuracy       map[string]float64 `json:"factorAccuracy"`
	FactorSamples        map[string]int     `json:"factorSamples"`
	StartWeights         Weights            `json:"startWeights"`
	FinalWeights         Weights            `json:"finalWeights"`
	BestCall             *PredictionRecord  `json:"bestCall,omitempty"`
	WorstMiss            *PredictionRecord  `json:"worstMiss,omitempty"`
}

// SeasonOf returns the year of a "2006-01-02" date, or 0 if unparseable.
func SeasonOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

// PickConfidence is how sure the engine was of the side it actually picked (0.5-1).
func (p PredictionRecord) PickConfidence() float64 {
	if p.Predicted == "W" {
		return p.WinProbability
	}
	return 1 - p.WinProbability
}

// IsScoredPrediction reports whether a record is a real, settled bot prediction.
func (p PredictionRecord) IsScoredPrediction() bool {
	return !p.Synthetic && p.Actual != "" && (p.Predicted == "W" || p.Predicted == "L")
}

// SummarizeSeason builds the scorecard for one season from the history.
// Synthetic (backfilled) records count toward the Rockies' W-L but not toward
// prediction accuracy.
func SummarizeSeason(records []PredictionRecord, season int, final Weights) SeasonSummary {
	sum := SeasonSummary{
		Season:       season,
		StartWeights: DefaultWeights(),
		FinalWeights: final,
	}

	var scored []PredictionRecord
	streak := 0
	for _, p := range records {
		if SeasonOf(p.Date) != season || p.Actual == "" {
			continue
		}
		if p.Actual == "W" {
			sum.RockiesWins++
		} else {
			sum.RockiesLosses++
		}
		if !p.IsScoredPrediction() {
			continue
		}
		scored = append(scored, p)
		sum.Predictions++
		if p.Predicted == "W" {
			sum.PickedWins++
		} else {
			sum.PickedLosses++
		}

		rec := p
		if p.Predicted == p.Actual {
			sum.Correct++
			streak++
			if streak > sum.LongestCorrectStreak {
				sum.LongestCorrectStreak = streak
			}
			if sum.BestCall == nil || rec.PickConfidence() > sum.BestCall.PickConfidence() {
				sum.BestCall = &rec
			}
		} else {
			streak = 0
			if sum.WorstMiss == nil || rec.PickConfidence() > sum.WorstMiss.PickConfidence() {
				sum.WorstMiss = &rec
			}
		}
	}

	if sum.Predictions > 0 {
		sum.Accuracy = math.Round(float64(sum.Correct)/float64(sum.Predictions)*1000) / 1000
	}
	sum.FactorAccuracy, sum.FactorSamples = CalculateFactorAccuracy(scored)
	return sum
}

// ArchiveAndReset moves every record from seasons before newSeason into
// archive/<season>/ (history + summary), then resets the live history to an
// empty slate with default weights. It is idempotent: if there is nothing from
// a prior season it does nothing and returns nil.
//
// Archive files are written before the live history is reset, so a crash at
// any point leaves the old records either in place or safely archived.
func ArchiveAndReset(h *PredictionHistory, dataDir string, newSeason int, now time.Time) (*SeasonSummary, error) {
	bySeason := map[int][]PredictionRecord{}
	var keep []PredictionRecord
	latest := 0
	for _, p := range h.Predictions {
		y := SeasonOf(p.Date)
		if y != 0 && y < newSeason {
			bySeason[y] = append(bySeason[y], p)
			if y > latest {
				latest = y
			}
			continue
		}
		keep = append(keep, p)
	}
	if latest == 0 {
		return nil, nil
	}

	var summary SeasonSummary
	for y, recs := range bySeason {
		// Only the most recent season ended with the live weights.
		weights := DefaultWeights()
		if y == latest {
			weights = h.Current
		}
		s := SummarizeSeason(recs, y, weights)
		s.ArchivedAt = now.UTC().Format(time.RFC3339)
		if y == latest {
			summary = s
		}
		if dataDir == "" {
			continue
		}
		dir := filepath.Join(dataDir, ArchiveDirName, strconv.Itoa(y))
		if err := writeJSON(filepath.Join(dir, historyFileName), PredictionHistory{Predictions: recs, Current: weights}); err != nil {
			return nil, fmt.Errorf("archiving %d history: %w", y, err)
		}
		if err := writeJSON(filepath.Join(dir, "summary.json"), s); err != nil {
			return nil, fmt.Errorf("archiving %d summary: %w", y, err)
		}
	}

	h.Predictions = keep
	h.Current = DefaultWeights()
	if err := SaveHistory(h, dataDir); err != nil {
		return nil, fmt.Errorf("saving fresh history: %w", err)
	}
	return &summary, nil
}

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data, 0644)
}
