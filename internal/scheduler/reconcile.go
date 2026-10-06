package scheduler

import (
	"fmt"

	"github.com/tlugger/rockiscope/internal/prediction"
)

// replyGraceDays: posted predictions this recent are left for the follow-up
// reply to settle (it only fires for unsettled records).
const replyGraceDays = 2

// reconcileTask checks the season's history against MLB once a day and fills
// in anything missing: games the bot never predicted (it wasn't running yet,
// or the Pi was down), and missing scores. It replaces running `backfill` by hand.
func (s *Scheduler) reconcileTask(season int, r *tickResult) {
	if s.season == nil || s.predHistory == nil {
		return
	}
	key := fmt.Sprintf("reconcile:%s", s.today())
	if s.done(key) {
		return
	}
	// Only reconcile a season the live history still holds (not one already
	// rolled into archive/), or the season in progress.
	holds := false
	for _, p := range s.predHistory.Predictions {
		if prediction.SeasonOf(p.Date) == season {
			holds = true
			break
		}
	}
	if !holds && season != s.denverNow().Year() {
		return
	}

	results, err := s.season.GetSeasonResultsFor(season)
	if err != nil {
		r.fail("history reconcile", err)
		return
	}
	protectFrom := s.denverNow().AddDate(0, 0, -replyGraceDays).Format("2006-01-02")
	res := prediction.Reconcile(s.predHistory, results, protectFrom)
	if res.Changed() {
		s.logger.Printf("reconciled %d history with MLB: %d missing games added, %d scores and %d results filled",
			season, res.Created, res.ScoresFilled, res.ActualsFilled)
		if err := s.savePredictionHistory(); err != nil {
			r.fail("saving reconciled history", err)
			return
		}
	}
	s.markDone(key, "")
}
