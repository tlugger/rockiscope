package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tlugger/rockiscope/internal/fsutil"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/seasonstate"
)

const seasonStateFileName = seasonstate.FileName

// Local names for the persisted types; the definitions live in seasonstate so
// the dashboard can read the same file.
type (
	pick        = seasonstate.Pick
	adoption    = seasonstate.Adoption
	openingDay  = seasonstate.OpeningDay
	seasonState = seasonstate.State
)

const (
	pickPostseason = seasonstate.PickPostseason
	pickSpring     = seasonstate.PickSpring
)

func newSeasonState() *seasonState { return seasonstate.New() }

func (s *Scheduler) seasonStatePath() string {
	return seasonstate.Path(s.dataDir)
}

func (s *Scheduler) loadSeasonState() {
	s.st = newSeasonState()
	if s.dataDir == "" {
		return
	}
	st, fromBackup, err := seasonstate.Read(s.dataDir)
	if os.IsNotExist(err) {
		s.logger.Println("no season state yet, starting fresh")
		return
	}
	if err != nil {
		// Both the file and its backup are unreadable. Keep them for forensics
		// and start over; idempotency keys are lost, so warn loudly.
		s.logger.Printf("WARNING: season state unreadable (%v); starting fresh", err)
		_ = os.Rename(s.seasonStatePath(), s.seasonStatePath()+".corrupt-"+s.now().Format("20060102-150405"))
		return
	}
	if fromBackup {
		s.logger.Println("warning: season state was damaged, recovered from backup")
	}
	s.st = st
}

// state returns the season state, creating an empty one if needed (tests build
// Schedulers as struct literals).
func (s *Scheduler) state() *seasonState {
	if s.st == nil {
		s.st = newSeasonState()
	}
	return s.st
}

func (s *Scheduler) saveSeasonState() {
	if s.dataDir == "" || s.dryRun {
		return
	}
	s.pruneState()
	data, err := json.MarshalIndent(s.state(), "", "  ")
	if err != nil {
		s.logger.Printf("warning: marshaling season state: %v", err)
		return
	}
	if err := fsutil.WriteFileAtomic(s.seasonStatePath(), data, 0644); err != nil {
		s.logger.Printf("warning: saving season state: %v", err)
	}
}

// pruneState drops bookkeeping old enough to be irrelevant so the file stays small.
func (s *Scheduler) pruneState() {
	st := s.state()
	cutoff := s.denverNow().AddDate(0, 0, -60).Format("2006-01-02")
	for day := range st.PostsByDay {
		if day < cutoff {
			delete(st.PostsByDay, day)
		}
	}
	for id, day := range st.TxnSeen {
		if day < cutoff {
			delete(st.TxnSeen, id)
		}
	}
}

func (s *Scheduler) done(key string) bool {
	_, ok := s.state().Done[key]
	return ok
}

// markDone records a key without a post (e.g. a missed pregame window).
func (s *Scheduler) markDone(key, uri string) {
	s.state().Done[key] = uri
	s.saveSeasonState()
}

func seasonKey(season int) string { return fmt.Sprint(season) }

func (s *Scheduler) denverNow() time.Time { return s.now().In(mlb.DenverLocation()) }
func (s *Scheduler) today() string        { return s.denverNow().Format("2006-01-02") }
