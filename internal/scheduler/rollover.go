package scheduler

import (
	"fmt"

	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// maybeRollover archives any prior season's predictions and weights to
// archive/<season>/ and resets the live history to defaults for newSeason.
// Safe to call on every wake: it does nothing once the history is current.
func (s *Scheduler) maybeRollover(newSeason int) error {
	if s.predHistory == nil || s.dryRun {
		return nil
	}
	sum, err := prediction.ArchiveAndReset(s.predHistory, s.dataDir, newSeason, s.now())
	if err != nil {
		return err
	}
	if sum == nil {
		return nil
	}
	s.logger.Printf("season rollover: archived %d (%d/%d correct), weights reset to defaults for %d",
		sum.Season, sum.Correct, sum.Predictions, newSeason)
	s.state().Rollovers[seasonKey(newSeason)] = *sum
	s.saveSeasonState()
	return nil
}

// rolloverTask performs the rollover, then announces it once.
func (s *Scheduler) rolloverTask(newSeason int, r *tickResult) {
	if err := s.maybeRollover(newSeason); err != nil {
		r.fail("season rollover", err)
		return
	}
	sum, ok := s.state().Rollovers[seasonKey(newSeason)]
	key := fmt.Sprintf("rollover:%d", newSeason)
	if !ok || sum.Predictions == 0 || s.done(key) || !s.chatterAllowed(r) {
		return
	}
	if _, err := s.postOnce(key, formatter.FormatRollover(sum, newSeason), nil, "", ""); err != nil {
		r.fail("rollover announcement", err)
	}
}
