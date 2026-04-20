package prediction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const historyFileName = "prediction_history.json"

var (
	defaultWeights = Weights{
		WinRate:   0.30,
		Pitcher:   0.30,
		H2H:       0.15,
		HomeAway:  0.10,
		Momentum: 0.05,
		Stars:     0.10,
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

func (h *PredictionHistory) completed() []PredictionRecord {
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
	for _, p := range h.completed() {
		if p.Predicted == p.Actual {
			count++
		}
	}
	return count
}

func (h *PredictionHistory) TotalCount() int {
	return len(h.completed())
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

func CalculateFactorAccuracy(predictions []PredictionRecord) map[string]float64 {
	factors := []string{"winRate", "pitcher", "h2h", "homeAway", "momentum", "stars"}
	accuracy := make(map[string]float64)
	correct := make(map[string]int)
	total := make(map[string]int)

	for _, p := range predictions {
		if p.Actual == "" {
			continue
		}

		direction := 1.0
		if p.Actual == "L" {
			direction = -1.0
		}

		fc := p.Factors
		if fc.WinRate != 0 {
			correctDir := (fc.WinRate*direction > 0.5)
			total["winRate"]++
			if correctDir == (p.Actual == p.Predicted) {
				correct["winRate"]++
			}
		}
		if fc.Pitcher != 0 {
			total["pitcher"]++
			if (fc.Pitcher > 0.5) == (p.Predicted == "W") {
				correct["pitcher"]++
			}
		}
		if fc.H2H != 0 {
			total["h2h"]++
			if (fc.H2H > 0.5) == (p.Predicted == "W") {
				correct["h2h"]++
			}
		}
		if fc.HomeAway != 0 {
			total["homeAway"]++
			if (fc.HomeAway > 0.5) == (p.Predicted == "W") {
				correct["homeAway"]++
			}
		}
		if fc.Momentum != 0 {
			total["momentum"]++
			if (fc.Momentum > 0.5) == (p.Predicted == "W") {
				correct["momentum"]++
			}
		}
		if fc.Stars != 0 {
			total["stars"]++
			if (fc.Stars > 0.5) == (p.Predicted == "W") {
				correct["stars"]++
			}
		}
	}

	for _, f := range factors {
		if total[f] == 0 {
			accuracy[f] = 0.5
		} else {
			accuracy[f] = float64(correct[f]) / float64(total[f])
		}
	}

	return accuracy
}

func AdjustWeights(current Weights, accuracy map[string]float64, n int) Weights {
	adjusted := current

	goodFactors := []string{}
	badFactors := []string{}

	for f, acc := range accuracy {
		if acc > 0.55 {
			goodFactors = append(goodFactors, f)
		} else if acc < 0.45 {
			badFactors = append(badFactors, f)
		}
	}

	shift := 0.03
	if len(goodFactors) > 0 && len(badFactors) > 0 {
		shiftPerFactor := shift / float64(len(goodFactors))
		for _, f := range goodFactors {
			switch f {
			case "winRate":
				adjusted.WinRate = min(0.45, adjusted.WinRate+shiftPerFactor)
			case "pitcher":
				adjusted.Pitcher = min(0.45, adjusted.Pitcher+shiftPerFactor)
			case "h2h":
				adjusted.H2H = min(0.25, adjusted.H2H+shiftPerFactor)
			case "homeAway":
				adjusted.HomeAway = min(0.20, adjusted.HomeAway+shiftPerFactor)
			case "momentum":
				adjusted.Momentum = min(0.15, adjusted.Momentum+shiftPerFactor)
			case "stars":
				adjusted.Stars = min(0.20, adjusted.Stars+shiftPerFactor)
			}
		}
		for _, f := range badFactors {
			switch f {
			case "winRate":
				adjusted.WinRate = max(0.10, adjusted.WinRate-shiftPerFactor)
			case "pitcher":
				adjusted.Pitcher = max(0.10, adjusted.Pitcher-shiftPerFactor)
			case "h2h":
				adjusted.H2H = max(0.02, adjusted.H2H-shiftPerFactor)
			case "homeAway":
				adjusted.HomeAway = max(0.02, adjusted.HomeAway-shiftPerFactor)
			case "momentum":
				adjusted.Momentum = max(0.01, adjusted.Momentum-shiftPerFactor)
			case "stars":
				adjusted.Stars = max(0.02, adjusted.Stars-shiftPerFactor)
			}
		}
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

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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