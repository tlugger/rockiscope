package scheduler

import (
	"fmt"

	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// reportCardTask posts the end-of-season thread once. Each post in the thread
// has its own key, so a failure mid-thread resumes from the missing post.
func (s *Scheduler) reportCardTask(season int, r *tickResult) {
	key := fmt.Sprintf("report:%d", season)
	if s.done(key) || s.predHistory == nil {
		return
	}

	sum := prediction.SummarizeSeason(s.predHistory.Predictions, season, s.predHistory.Current)
	if sum.Predictions == 0 {
		s.logger.Printf("no %d predictions on file, skipping report card", season)
		s.markDone(key, "")
		return
	}
	if !s.chatterAllowed(r) {
		return
	}

	// The history can be missing games; official standings win when available.
	if rec, err := s.season.GetTeamRecordFor(mlb.RockiesID, season); err == nil && rec.Wins+rec.Losses > 0 {
		sum.RockiesWins, sum.RockiesLosses = rec.Wins, rec.Losses
	} else if err != nil {
		s.logger.Printf("warning: standings unavailable for report card, using history: %v", err)
	}

	root, parent := "", ""
	for i, text := range formatter.ReportCard(sum) {
		uri, err := s.postOnce(fmt.Sprintf("%s:%d", key, i), text, nil, parent, root)
		if err != nil {
			r.fail("report card", err)
			return
		}
		if i == 0 {
			root = uri
		}
		parent = uri
	}
	s.markDone(key, root)
}
