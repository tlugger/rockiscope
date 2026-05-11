package prediction

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const historyFileName = "prediction_history.json"

var (
	defaultWeights = Weights{
		WinRate:  0.25,
		Pitcher:  0.35,
		H2H:     0.05,
		HomeAway: 0.15,
		Momentum: 0.10,
		Stars:    0.10,
	}
)

func DefaultWeights() Weights {
	return defaultWeights
}

type Weights struct {
	WinRate   float64 `json:"winRate"`
	Pitcher  float64 `json:"pitcher"`
	H2H      float64 `json:"h2h"`
	HomeAway float64 `json:"homeAway"`
	Momentum float64 `json:"momentum"`
	Stars    float64 `json:"stars"`
}

type FactorScores struct {
	WinRate   float64 `json:"winRate,omitempty"`
	Pitcher  float64 `json:"pitcher,omitempty"`
	H2H      float64 `json:"h2h,omitempty"`
	HomeAway float64 `json:"homeAway,omitempty"`
	Momentum float64 `json:"momentum,omitempty"`
	Stars    float64 `json:"stars,omitempty"`
}

func (w Weights) total() float64 {
	return w.WinRate + w.Pitcher + w.H2H + w.HomeAway + w.Momentum + w.Stars
}

type PredictionRecord struct {
	Date          string        `json:"date"`
	Opponent      string        `json:"opponent"`
	IsHome        bool          `json:"isHome"`
	Predicted     string        `json:"predicted"` // "W" or "L"
	Confidence   float64       `json:"confidence"` // 0-100
	Actual       string        `json:"actual,omitempty"` // "W" or "L" or ""
	RockiesScore int          `json:"rockiesScore,omitempty"`
	OppScore     int          `json:"oppScore,omitempty"`
	Synthetic    bool         `json:"synthetic,omitempty"` // backfilled pre-bot record; excluded from prediction-accuracy stats
	PostURI       string        `json:"postUri,omitempty"`
	GamePK       int          `json:"gamePk,omitempty"`
	Factors      FactorScores  `json:"factors"`
	WinProbability float64    `json:"winProbability"`
}

type PredictionHistory struct {
	Predictions []PredictionRecord `json:"predictions"`
	Current    Weights            `json:"currentWeights"`
}

func (h *PredictionHistory) Add(pred PredictionRecord) {
	h.Predictions = append(h.Predictions, pred)
	if len(h.Predictions) > 50 {
		h.Predictions = h.Predictions[len(h.Predictions)-50:]
	}
}

func (h *PredictionHistory) Recent(n int) []PredictionRecord {
	if n > len(h.Predictions) {
		n = len(h.Predictions)
	}
	if n == 0 {
		return nil
	}
	return h.Predictions[len(h.Predictions)-n:]
}

func (h *PredictionHistory) Completed() []PredictionRecord {
	var completed []PredictionRecord
	for _, p := range h.Predictions {
		if p.Actual != "" {
			completed = append(completed, p)
		}
	}
	return completed
}

func (h *PredictionHistory) CorrectCount() int {
	count := 0
	for _, p := range h.Completed() {
		if p.Predicted == p.Actual {
			count++
		}
	}
	return count
}

func (h *PredictionHistory) TotalCount() int {
	return len(h.Completed())
}

func LoadHistory(dataDir string) (*PredictionHistory, error) {
	if dataDir == "" {
		return &PredictionHistory{
			Predictions: nil,
			Current:     defaultWeights,
		}, nil
	}

	path := filepath.Join(dataDir, historyFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &PredictionHistory{
			Predictions: nil,
			Current:     defaultWeights,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading prediction history: %w", err)
	}

	var hist PredictionHistory
	if err := json.Unmarshal(data, &hist); err != nil {
		return nil, fmt.Errorf("parsing prediction history: %w", err)
	}

	if hist.Current.total() <= 0 {
		hist.Current = defaultWeights
	}

	return &hist, nil
}

func SaveHistory(h *PredictionHistory, dataDir string) error {
	if dataDir == "" {
		return nil
	}

	path := filepath.Join(dataDir, historyFileName)
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling history: %w", err)
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing history: %w", err)
	}
	return nil
}

// CalculateFactorAccuracy checks whether each factor's score (>0.5 or <0.5)
// aligned with the actual game outcome, not the prediction.
// Returns accuracy (0-1) and sample counts per factor.
func CalculateFactorAccuracy(predictions []PredictionRecord) (accuracy map[string]float64, sampleCounts map[string]int) {
	factors := []string{"winRate", "pitcher", "h2h", "homeAway", "momentum", "stars"}
	accuracy = make(map[string]float64)
	sampleCounts = make(map[string]int)
	correct := make(map[string]int)

	for _, p := range predictions {
		if p.Actual == "" || p.Synthetic {
			continue
		}
		won := p.Actual == "W"
		fc := p.Factors

		check := func(name string, score float64) {
			sampleCounts[name]++
			if (score > 0.5) == won {
				correct[name]++
			}
		}

		if fc.WinRate != 0 {
			check("winRate", fc.WinRate)
		}
		if fc.Pitcher != 0 {
			check("pitcher", fc.Pitcher)
		}
		if fc.H2H != 0 {
			check("h2h", fc.H2H)
		}
		if fc.HomeAway != 0 {
			check("homeAway", fc.HomeAway)
		}
		if fc.Momentum != 0 {
			check("momentum", fc.Momentum)
		}
		if fc.Stars != 0 {
			check("stars", fc.Stars)
		}
	}

	for _, f := range factors {
		if sampleCounts[f] == 0 {
			accuracy[f] = 0.5
		} else {
			accuracy[f] = float64(correct[f]) / float64(sampleCounts[f])
		}
	}

	return accuracy, sampleCounts
}

// AdjustWeights updates weights using a conservative approach:
//   - Decaying learning rate: smaller adjustments as more games are seen
//   - Regularization: pulls weights back toward defaults to prevent runaway drift
//   - Minimum sample: only adjusts factors with enough observations
//
// accuracy maps factor names to their accuracy (0-1) against actual outcomes.
// sampleCounts maps factor names to how many games had that factor available.
// totalGames is the cumulative number of completed games (used to decay the learning rate).
func AdjustWeights(current Weights, accuracy map[string]float64, sampleCounts map[string]int, totalGames int) Weights {
	if totalGames < 5 {
		return current
	}

	learningRate := 0.15 / math.Sqrt(float64(totalGames))
	regularization := 0.02
	minSamples := 5

	type factorCfg struct {
		name     string
		get      func(Weights) float64
		set      func(*Weights, float64)
		floor    float64
		ceiling  float64
	}

	factors := []factorCfg{
		{"winRate", func(w Weights) float64 { return w.WinRate }, func(w *Weights, v float64) { w.WinRate = v }, 0.10, 0.40},
		{"pitcher", func(w Weights) float64 { return w.Pitcher }, func(w *Weights, v float64) { w.Pitcher = v }, 0.10, 0.45},
		{"h2h", func(w Weights) float64 { return w.H2H }, func(w *Weights, v float64) { w.H2H = v }, 0.02, 0.15},
		{"homeAway", func(w Weights) float64 { return w.HomeAway }, func(w *Weights, v float64) { w.HomeAway = v }, 0.05, 0.25},
		{"momentum", func(w Weights) float64 { return w.Momentum }, func(w *Weights, v float64) { w.Momentum = v }, 0.03, 0.20},
		{"stars", func(w Weights) float64 { return w.Stars }, func(w *Weights, v float64) { w.Stars = v }, 0.03, 0.15},
	}

	adjusted := current
	defaults := defaultWeights

	for _, f := range factors {
		cur := f.get(adjusted)
		def := f.get(defaults)

		acc, hasAcc := accuracy[f.name]
		samples := sampleCounts[f.name]

		if !hasAcc || samples < minSamples {
			// Not enough data — regularize toward default only
			cur += regularization * (def - cur)
		} else {
			// Shift proportional to how far accuracy deviates from 0.5
			signal := (acc - 0.5) * 2 // range [-1, 1]
			cur += learningRate * signal

			// Regularize toward default
			cur += regularization * (def - cur)
		}

		cur = math.Max(f.floor, math.Min(f.ceiling, cur))
		f.set(&adjusted, cur)
	}

	total := adjusted.total()
	if total > 0 {
		adjusted.WinRate /= total
		adjusted.Pitcher /= total
		adjusted.H2H /= total
		adjusted.HomeAway /= total
		adjusted.Momentum /= total
		adjusted.Stars /= total
	}

	return adjusted
}


type byDate []PredictionRecord

func (a byDate) Len() int           { return len(a) }
func (a byDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byDate) Less(i, j int) bool {
	if a[i].Date == "" && a[j].Date == "" {
		return false
	}
	if a[i].Date == "" {
		return false
	}
	if a[j].Date == "" {
		return true
	}
	return a[i].Date < a[j].Date
}

func sortByDate(records []PredictionRecord) {
	sort.Sort(byDate(records))
}

func init() {
	sort.Sort(byDate(nil))
}