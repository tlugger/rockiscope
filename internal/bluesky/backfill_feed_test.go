package bluesky

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return tt
}

func predPost(uri, createdAt, text string) AuthorFeedItem {
	return AuthorFeedItem{
		PostURI:      uri,
		CreatedAt:    createdAt[:10],
		CreatedAtRaw: createdAt,
		Text:         text,
	}
}

func gameText(vsAt, opponent, pick string, pct int) string {
	verb := "victory"
	if pick == "L" {
		verb = "defeat"
	}
	return "⚾ Rockies " + vsAt + " " + opponent +
		"\n🕐 6:40 PM MDT at Coors Field\n\n🔮 The stars lean toward a Rockies " + verb + " (" + strconv.Itoa(pct) + "%)"
}

func findByPk(hist *prediction.PredictionHistory, pk int) *prediction.PredictionRecord {
	for i := range hist.Predictions {
		if hist.Predictions[i].GamePK == pk {
			return &hist.Predictions[i]
		}
	}
	return nil
}

// End-to-end backfill over two real-world shapes at once:
//   - date drift: a post whose UTC timestamp is the day after the official date
//   - split double-header: two posts one day, one before each game, distinct pks
func TestBackfillFromFeed_HandlesDriftAndDoubleHeader(t *testing.T) {
	games := []mlb.GameResult{
		// Away night game: official date 04-09, first pitch rolls to 04-10 UTC.
		{GamePk: 823319, Date: "2026-04-09", GameDateTime: mustTime(t, "2026-04-10T01:40:00Z"), Opponent: "San Diego Padres", IsHome: false, RockiesScore: 3, OppScore: 7, Won: false},
		// Split double-header 05-07: game 1 at 1:10 PM MDT, game 2 at 6:40 PM MDT
		// (official date 05-07 for both; game 2's first pitch rolls to 05-08 UTC).
		{GamePk: 824361, Date: "2026-05-07", GameDateTime: mustTime(t, "2026-05-07T19:10:00Z"), Opponent: "New York Mets", IsHome: true, GameNumber: 1, RockiesScore: 5, OppScore: 10, Won: false},
		{GamePk: 824362, Date: "2026-05-07", GameDateTime: mustTime(t, "2026-05-08T00:40:00Z"), Opponent: "New York Mets", IsHome: true, GameNumber: 2, RockiesScore: 6, OppScore: 2, Won: true},
	}

	items := []AuthorFeedItem{
		// Padres: posted ~1h before first pitch, UTC date is 04-10 (drift).
		predPost("at://padres", "2026-04-10T00:40:00Z", gameText("@", "San Diego Padres", "L", 60)),
		// DH game 1 post (~1h before 1:10 PM game).
		predPost("at://mets1", "2026-05-07T18:10:00Z", gameText("vs", "New York Mets", "L", 61)),
		// DH game 2 post (~1h before 6:40 PM game).
		predPost("at://mets2", "2026-05-07T23:40:00Z", gameText("vs", "New York Mets", "W", 55)),
		// A reply (follow-up) that must be ignored.
		{PostURI: "at://reply", CreatedAt: "2026-05-07", CreatedAtRaw: "2026-05-07T22:00:00Z", Text: "🌙 Planets aligned.", IsReply: true},
	}

	hist := &prediction.PredictionHistory{Current: prediction.Weights{WinRate: 1}}
	n := backfillFromFeed(hist, items, games, testLogger())

	if n != 3 {
		t.Fatalf("expected 3 predictions backfilled, got %d", n)
	}
	if len(hist.Predictions) != 3 {
		t.Fatalf("expected 3 records, got %d", len(hist.Predictions))
	}

	padres := findByPk(hist, 823319)
	if padres == nil {
		t.Fatal("padres record missing")
	}
	if padres.Date != "2026-04-09" {
		t.Errorf("padres date = %q, want official 2026-04-09 (drift corrected)", padres.Date)
	}
	if padres.Predicted != "L" || math.Abs(padres.WinProbability-0.40) > 0.001 {
		t.Errorf("padres pred=%q winProb=%f, want L / 0.40 (defeat 60%%)", padres.Predicted, padres.WinProbability)
	}

	g1 := findByPk(hist, 824361)
	g2 := findByPk(hist, 824362)
	if g1 == nil || g2 == nil {
		t.Fatal("double-header records missing")
	}
	if g1.PostURI != "at://mets1" || g1.Predicted != "L" || g1.GameNumber != 1 {
		t.Errorf("DH game 1 wrong: uri=%q pred=%q gn=%d", g1.PostURI, g1.Predicted, g1.GameNumber)
	}
	if g2.PostURI != "at://mets2" || g2.Predicted != "W" || g2.GameNumber != 2 {
		t.Errorf("DH game 2 wrong: uri=%q pred=%q gn=%d", g2.PostURI, g2.Predicted, g2.GameNumber)
	}
	// Game 2's UTC first pitch is 05-08, but the record uses the official date.
	if g2.Date != "2026-05-07" {
		t.Errorf("DH game 2 date = %q, want official 2026-05-07", g2.Date)
	}
}

// When two posts resolve to the same game (a re-post after a postponement), the
// post closest to first pitch wins and no duplicate record is created.
func TestBackfillFromFeed_DedupesRepostByNearestFirstPitch(t *testing.T) {
	games := []mlb.GameResult{
		{GamePk: 824362, Date: "2026-05-07", GameDateTime: mustTime(t, "2026-05-07T19:10:00Z"), Opponent: "New York Mets", IsHome: true, RockiesScore: 6, OppScore: 2, Won: true},
	}
	items := []AuthorFeedItem{
		// Makeup-day post ~1h before first pitch — should win.
		predPost("at://makeup", "2026-05-07T18:14:00Z", gameText("vs", "New York Mets", "L", 61)),
		// Earlier same-day post also resolves to this game but is farther away.
		predPost("at://early", "2026-05-07T06:00:00Z", gameText("vs", "New York Mets", "W", 55)),
	}

	hist := &prediction.PredictionHistory{Current: prediction.Weights{WinRate: 1}}
	n := backfillFromFeed(hist, items, games, testLogger())

	if n != 1 || len(hist.Predictions) != 1 {
		t.Fatalf("expected 1 deduped record, got n=%d len=%d", n, len(hist.Predictions))
	}
	rec := hist.Predictions[0]
	if rec.PostURI != "at://makeup" || rec.Predicted != "L" {
		t.Errorf("expected nearest-first-pitch post to win, got uri=%q pred=%q", rec.PostURI, rec.Predicted)
	}
}

// Backfill upserts onto an existing record (e.g. a synthetic one) by gamePk
// rather than appending a duplicate, and clears the synthetic flag.
func TestBackfillFromFeed_UpsertsExistingByGamePk(t *testing.T) {
	games := []mlb.GameResult{
		{GamePk: 824210, Date: "2026-04-14", GameDateTime: mustTime(t, "2026-04-15T01:40:00Z"), Opponent: "Houston Astros", IsHome: false, RockiesScore: 6, OppScore: 7, Won: false},
	}
	hist := &prediction.PredictionHistory{
		Predictions: []prediction.PredictionRecord{
			{Date: "2026-04-15", Opponent: "Houston Astros", Predicted: "L", GamePK: 824210, Synthetic: true, Actual: "L", RockiesScore: 6, OppScore: 7, WinProbability: 0.5, Confidence: 50},
		},
		Current: prediction.Weights{WinRate: 1},
	}
	items := []AuthorFeedItem{
		predPost("at://houston", "2026-04-15T00:40:00Z", gameText("@", "Houston Astros", "L", 58)),
	}

	n := backfillFromFeed(hist, items, games, testLogger())
	if n != 1 {
		t.Fatalf("expected 1 update, got %d", n)
	}
	if len(hist.Predictions) != 1 {
		t.Fatalf("expected upsert (no new record), got %d records", len(hist.Predictions))
	}
	rec := hist.Predictions[0]
	if rec.Synthetic {
		t.Error("synthetic flag should be cleared after backfill")
	}
	if rec.PostURI != "at://houston" {
		t.Errorf("PostURI = %q, want at://houston", rec.PostURI)
	}
	if rec.Date != "2026-04-14" {
		t.Errorf("Date = %q, want official 2026-04-14 (drift corrected)", rec.Date)
	}
	// winProb corrected to P(win) = 1 - 0.58, and the pre-existing result kept.
	if math.Abs(rec.WinProbability-0.42) > 0.001 {
		t.Errorf("winProb = %f, want 0.42", rec.WinProbability)
	}
	if rec.Actual != "L" {
		t.Errorf("existing result should be preserved, got actual=%q", rec.Actual)
	}
}

// A post with no game inside the match window is skipped, not force-matched.
func TestBackfillFromFeed_SkipsWhenNoGameInWindow(t *testing.T) {
	games := []mlb.GameResult{
		{GamePk: 1, Date: "2026-06-01", GameDateTime: mustTime(t, "2026-06-01T20:00:00Z"), Opponent: "Los Angeles Angels", IsHome: false, Won: true},
	}
	items := []AuthorFeedItem{
		// Different opponent -> no match.
		predPost("at://x", "2026-06-01T19:00:00Z", gameText("@", "Seattle Mariners", "L", 55)),
	}
	hist := &prediction.PredictionHistory{Current: prediction.Weights{WinRate: 1}}
	if n := backfillFromFeed(hist, items, games, testLogger()); n != 0 {
		t.Errorf("expected 0 backfilled, got %d", n)
	}
	if len(hist.Predictions) != 0 {
		t.Errorf("expected no records created, got %d", len(hist.Predictions))
	}
}
