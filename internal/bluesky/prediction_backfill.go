package bluesky

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

var (
	opponentRe = regexp.MustCompile(`^⚾ Rockies (?:vs |@ )(.+)`)
	pickRe     = regexp.MustCompile(`victory|defeat`)
	pctRe      = regexp.MustCompile(`\((\d+)%\)$`)
	offDayRe   = regexp.MustCompile(`^⚾ No Rockies game today`)
)

// matchWindow bounds how far a prediction post's timestamp may sit from a game's
// first pitch to be considered the same game. Posts go up ~1h before first pitch;
// the generous window also lets a prediction posted the day before a postponed
// game bind to its next-day makeup, while still excluding the following series.
const matchWindow = 30 * time.Hour

// ParsedPrediction holds data extracted from a Bluesky prediction post.
type ParsedPrediction struct {
	Date      string
	Opponent  string
	Predicted string  // "W" or "L"
	WinProb   float64 // P(Rockies win), 0-1
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

	// The post shows the probability of the PREDICTED outcome. WinProbability is
	// always P(Rockies win): for a victory pick that is the shown %, but for a
	// defeat pick it is the complement. (The old code stored the defeat % as the
	// win probability, inverting every "L" record.)
	winProb := pct / 100
	if predicted == "L" {
		winProb = 1 - pct/100
	}

	return &ParsedPrediction{
		Date:      date,
		Opponent:  opponent,
		Predicted: predicted,
		WinProb:   winProb,
	}
}

// nearestGame returns the game most likely referenced by a prediction post: the
// same-opponent game whose first pitch is closest to the post time, within
// matchWindow. The absolute distance to first pitch is returned for dedup.
func nearestGame(opponent string, postTime time.Time, games []mlb.GameResult) (*mlb.GameResult, time.Duration) {
	var best *mlb.GameResult
	bestDelta := time.Duration(math.MaxInt64)
	for i := range games {
		g := &games[i]
		if g.Opponent != opponent || g.GameDateTime.IsZero() {
			continue
		}
		delta := g.GameDateTime.Sub(postTime)
		if delta < 0 {
			delta = -delta
		}
		if delta <= matchWindow && delta < bestDelta {
			best, bestDelta = g, delta
		}
	}
	return best, bestDelta
}

// BackfillPredictionsFromBluesky fetches the bot's Bluesky feed, resolves each
// prediction post to its MLB game by first-pitch time, and upserts the record
// keyed on gamePk. This correctly handles double-headers (two games one date)
// and postponements (a prediction that lands on a makeup on another day). When
// two posts resolve to the same game (a re-post of a postponed prediction), the
// post nearest to first pitch wins. Game results already present on records are
// left untouched.
//
// Limitation: matching is purely by first-pitch time, so a prediction posted a
// day or more before a game that is later made up as game 2 of a *straight*
// double-header (whose two games share a near-identical placeholder start time)
// can be ambiguous. The live path avoids this entirely by recording gamePk at
// post time; this backfill is a recovery tool for reconstructing lost history.
func BackfillPredictionsFromBluesky(hist *prediction.PredictionHistory, actor string, games []mlb.GameResult, logger *log.Logger) (int, error) {
	logger.Println("fetching posts from Bluesky...")
	items, err := GetAuthorFeed(actor)
	if err != nil {
		return 0, fmt.Errorf("fetching Bluesky feed: %w", err)
	}
	logger.Printf("fetched %d posts", len(items))
	return backfillFromFeed(hist, items, games, logger), nil
}

// backfillFromFeed is the testable core of the Bluesky backfill: it takes already
// fetched feed items rather than reaching out to the network.
func backfillFromFeed(hist *prediction.PredictionHistory, items []AuthorFeedItem, games []mlb.GameResult, logger *log.Logger) int {
	// Resolve every prediction post to a game, keeping the closest post per game.
	type match struct {
		pp      *ParsedPrediction
		game    *mlb.GameResult
		postURI string
		delta   time.Duration
	}
	byGame := make(map[int]match)
	for _, item := range items {
		if item.IsReply {
			continue
		}
		pp := ParsePredictionPost(item.Text, item.CreatedAt)
		if pp == nil {
			continue
		}
		postTime, err := time.Parse(time.RFC3339, item.CreatedAtRaw)
		if err != nil {
			logger.Printf("skipping post with unparseable timestamp %q", item.CreatedAtRaw)
			continue
		}
		g, delta := nearestGame(pp.Opponent, postTime, games)
		if g == nil {
			logger.Printf("no MLB game within window for %s post vs %s (%s)", pp.Date, pp.Opponent, item.PostURI)
			continue
		}
		if prev, ok := byGame[g.GamePk]; !ok || delta < prev.delta {
			byGame[g.GamePk] = match{pp: pp, game: g, postURI: item.PostURI, delta: delta}
		}
	}

	// Index existing records by gamePk for upsert.
	byGamePk := make(map[int]*prediction.PredictionRecord)
	for i := range hist.Predictions {
		p := &hist.Predictions[i]
		if p.GamePK != 0 {
			byGamePk[p.GamePK] = p
		}
	}

	var updated int
	for pk, m := range byGame {
		g := m.game
		rec := byGamePk[pk]
		if rec == nil {
			hist.Predictions = append(hist.Predictions, prediction.PredictionRecord{GamePK: pk})
			rec = &hist.Predictions[len(hist.Predictions)-1]
			byGamePk[pk] = rec
		}

		rec.Date = g.Date
		rec.Opponent = g.Opponent
		rec.IsHome = g.IsHome
		rec.GameNumber = g.GameNumber
		rec.Predicted = m.pp.Predicted
		rec.WinProbability = m.pp.WinProb
		rec.Confidence = m.pp.WinProb * 100
		rec.PostURI = m.postURI
		rec.Synthetic = false
		updated++
	}

	logger.Printf("backfilled %d predictions from Bluesky posts", updated)
	return updated
}
