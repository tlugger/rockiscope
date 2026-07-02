package bluesky

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/tlugger/rockiscope/internal/prediction"
)

var (
	opponentRe = regexp.MustCompile(`^⚾ Rockies (?:vs |@ )(.+)`)
	pickRe     = regexp.MustCompile(`victory|defeat`)
	pctRe      = regexp.MustCompile(`\((\d+)%\)$`)
	offDayRe   = regexp.MustCompile(`^⚾ No Rockies game today`)
)

// ParsedPrediction holds data extracted from a Bluesky prediction post.
type ParsedPrediction struct {
	Date      string
	Opponent  string
	Predicted string // "W" or "L"
	WinProb   float64 // 0-1
}

// ParsePredictionPost attempts to parse a Bluesky post text as a game prediction.
// Returns nil if the post is not a prediction (off-day, reply, etc.).
func ParsePredictionPost(text string, date string) *ParsedPrediction {
	if offDayRe.MatchString(text) {
		return nil
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return nil
	}

	m := opponentRe.FindStringSubmatch(lines[0])
	if m == nil {
		return nil
	}
	opponent := strings.TrimSpace(m[1])

	var predLine string
	for _, line := range lines {
		if strings.Contains(line, "🔮") {
			predLine = line
			break
		}
	}
	if predLine == "" {
		return nil
	}

	pickMatch := pickRe.FindString(predLine)
	var predicted string
	switch pickMatch {
	case "victory":
		predicted = "W"
	case "defeat":
		predicted = "L"
	default:
		return nil
	}

	pctMatch := pctRe.FindStringSubmatch(predLine)
	if pctMatch == nil {
		return nil
	}
	pct, err := strconv.ParseFloat(pctMatch[1], 64)
	if err != nil {
		return nil
	}

	return &ParsedPrediction{
		Date:      date,
		Opponent:  opponent,
		Predicted: predicted,
		WinProb:   pct / 100,
	}
}

// BackfillPredictionsFromBluesky fetches the bot's Bluesky feed, parses prediction
// posts, and updates synthetic records in history with the real prediction data.
// Factors and game results (Actual, RockiesScore, OppScore) are left untouched.
func BackfillPredictionsFromBluesky(hist *prediction.PredictionHistory, actor string, logger *log.Logger) (int, error) {
	logger.Println("fetching posts from Bluesky...")
	items, err := GetAuthorFeed(actor)
	if err != nil {
		return 0, fmt.Errorf("fetching Bluesky feed: %w", err)
	}
	logger.Printf("fetched %d posts", len(items))

	var updated int
	for _, item := range items {
		if item.IsReply {
			continue
		}

		pp := ParsePredictionPost(item.Text, item.CreatedAt)
		if pp == nil {
			continue
		}

		for i := range hist.Predictions {
			p := &hist.Predictions[i]
			if !p.Synthetic {
				continue
			}
			if p.Date != pp.Date || p.Opponent != pp.Opponent {
				continue
			}

			p.Predicted = pp.Predicted
			p.WinProbability = pp.WinProb
			p.Confidence = pp.WinProb * 100
			p.Synthetic = false
			updated++
			break
		}
	}

	logger.Printf("backfilled %d predictions from Bluesky posts", updated)
	return updated, nil
}
