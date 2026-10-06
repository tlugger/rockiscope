package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/fsutil"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

const seasonStateFileName = "season_state.json"

// pick is a prediction made outside the regular season (postseason adoption
// or spring training). These never touch prediction_history.json, so they
// can't skew the real season's stats or the learned weights.
type pick struct {
	Kind           string    `json:"kind"` // pickPostseason | pickSpring
	Season         int       `json:"season"`
	GamePk         int       `json:"gamePk"`
	Date           string    `json:"date"`
	TeamID         int       `json:"teamId"`
	TeamName       string    `json:"teamName"`
	Opponent       string    `json:"opponent"`
	Pick           string    `json:"pick"` // "W"/"L" from TeamID's perspective
	WinProbability float64   `json:"winProbability"`
	PostURI        string    `json:"postUri,omitempty"`
	Result         string    `json:"result,omitempty"` // "W"/"L" once final, "void" if never played
	Score          string    `json:"score,omitempty"`
	FirstPitch     time.Time `json:"firstPitch"`
}

const (
	pickPostseason = "postseason"
	pickSpring     = "spring"
)

type adoption struct {
	Season       int                 `json:"season"`
	TeamID       int                 `json:"teamId"`
	TeamName     string              `json:"teamName"`
	AdoptedOn    string              `json:"adoptedOn"`
	EliminatedOn string              `json:"eliminatedOn,omitempty"`
	Reading      astro.RosterReading `json:"reading"`
}

type openingDay struct {
	Date     string `json:"date"`
	Opponent string `json:"opponent,omitempty"`
	IsHome   bool   `json:"isHome"`
}

// seasonState is everything the off-season modes need to survive a restart.
// Every post is recorded in Done under an idempotency key the moment it
// succeeds, so a reboot mid-day (or mid-thread) resumes without double-posting.
type seasonState struct {
	Done               map[string]string              `json:"done"` // key -> post URI ("" when deliberately skipped)
	PostsByDay         map[string]int                 `json:"postsByDay"`
	SeasonDates        map[string]mlb.SeasonDates     `json:"seasonDates"`
	SeasonDatesFetched string                         `json:"seasonDatesFetched,omitempty"`
	OpeningDays        map[string]openingDay          `json:"openingDays"`
	Readings           map[string]astro.RosterReading `json:"readings"` // "season:teamID"
	Adoptions          []adoption                     `json:"adoptions"`
	Champions          map[string]string              `json:"champions"` // season -> champion
	ChampionDates      map[string]string              `json:"championDates"`
	Picks              []pick                         `json:"picks"`
	TxnSeen            map[string]string              `json:"txnSeen"` // transaction id -> date
	HotStoveSince      string                         `json:"hotStoveSince,omitempty"`
	// Rollovers holds the summary of each archived season, keyed by the new season.
	Rollovers map[string]prediction.SeasonSummary `json:"rollovers"`
}

func newSeasonState() *seasonState {
	st := &seasonState{}
	st.init()
	return st
}

func (st *seasonState) init() {
	if st.Done == nil {
		st.Done = map[string]string{}
	}
	if st.PostsByDay == nil {
		st.PostsByDay = map[string]int{}
	}
	if st.SeasonDates == nil {
		st.SeasonDates = map[string]mlb.SeasonDates{}
	}
	if st.OpeningDays == nil {
		st.OpeningDays = map[string]openingDay{}
	}
	if st.Readings == nil {
		st.Readings = map[string]astro.RosterReading{}
	}
	if st.Champions == nil {
		st.Champions = map[string]string{}
	}
	if st.ChampionDates == nil {
		st.ChampionDates = map[string]string{}
	}
	if st.TxnSeen == nil {
		st.TxnSeen = map[string]string{}
	}
	if st.Rollovers == nil {
		st.Rollovers = map[string]prediction.SeasonSummary{}
	}
}

func (s *Scheduler) seasonStatePath() string {
	return filepath.Join(s.dataDir, seasonStateFileName)
}

func (s *Scheduler) loadSeasonState() {
	s.st = newSeasonState()
	if s.dataDir == "" {
		return
	}
	data, fromBackup, err := fsutil.ReadFileWithFallback(s.seasonStatePath(), func(b []byte) error {
		var probe seasonState
		return json.Unmarshal(b, &probe)
	})
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
	if err := json.Unmarshal(data, s.st); err != nil {
		s.logger.Printf("warning: parsing season state: %v", err)
		s.st = newSeasonState()
		return
	}
	s.st.init()
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
