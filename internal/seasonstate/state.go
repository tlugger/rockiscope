// Package seasonstate defines season_state.json: everything the postseason,
// offseason and spring modes persist. The scheduler is its only writer; the
// dashboard reads it.
package seasonstate

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/fsutil"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

const FileName = "season_state.json"

const (
	PickPostseason = "postseason"
	PickSpring     = "spring"
)

// Pick is a prediction made outside the regular season (postseason adoption
// or spring training). These never touch prediction_history.json, so they
// can't skew the real season's stats or the learned weights. Scores and
// factors are from TeamID's perspective.
type Pick struct {
	Kind           string                  `json:"kind"` // PickPostseason | PickSpring
	Season         int                     `json:"season"`
	GamePk         int                     `json:"gamePk"`
	Date           string                  `json:"date"`
	TeamID         int                     `json:"teamId"`
	TeamName       string                  `json:"teamName"`
	Opponent       string                  `json:"opponent"`
	IsHome         bool                    `json:"isHome"`
	Pick           string                  `json:"pick"` // "W"/"L"
	WinProbability float64                 `json:"winProbability"`
	Factors        prediction.FactorScores `json:"factors"`
	Series         string                  `json:"series,omitempty"`       // "NLDS Game 3"
	SeriesResult   string                  `json:"seriesResult,omitempty"` // after the game, "MIL wins 3-1"
	PostURI        string                  `json:"postUri,omitempty"`
	Result         string                  `json:"result,omitempty"` // "W"/"L" once final, "void" if never played
	Score          string                  `json:"score,omitempty"`  // "5-3"
	TeamScore      int                     `json:"teamScore,omitempty"`
	OppScore       int                     `json:"oppScore,omitempty"`
	FirstPitch     time.Time               `json:"firstPitch"`
}

type Adoption struct {
	Season       int                 `json:"season"`
	TeamID       int                 `json:"teamId"`
	TeamName     string              `json:"teamName"`
	AdoptedOn    string              `json:"adoptedOn"`
	EliminatedOn string              `json:"eliminatedOn,omitempty"`
	Reading      astro.RosterReading `json:"reading"`
}

type OpeningDay struct {
	Date     string `json:"date"`
	Opponent string `json:"opponent,omitempty"`
	IsHome   bool   `json:"isHome"`
}

// HotStoveEntry is a roster move the bot posted about.
type HotStoveEntry struct {
	Season      int    `json:"season"` // the season whose offseason this was
	Date        string `json:"date"`
	ID          int    `json:"id"`
	TypeCode    string `json:"typeCode"`
	TypeDesc    string `json:"typeDesc,omitempty"`
	Description string `json:"description"`
	Player      string `json:"player,omitempty"`
	Sign        string `json:"sign,omitempty"`
	Arriving    bool   `json:"arriving"`
	// Compatibility is the player's 0-3 Cancer compatibility (-1 when unknown).
	Compatibility int    `json:"compatibility"`
	PostURI       string `json:"postUri,omitempty"`
	Digest        bool   `json:"digest,omitempty"` // posted as part of a roundup
}

// Status is the scheduler's most recent view of the calendar.
type Status struct {
	Phase     string `json:"phase"`
	Season    int    `json:"season"`
	UpdatedAt string `json:"updatedAt"`
}

// State is everything the off-season modes need to survive a restart.
// Every post is recorded in Done under an idempotency key the moment it
// succeeds, so a reboot mid-day (or mid-thread) resumes without double-posting.
type State struct {
	Done               map[string]string              `json:"done"` // key -> post URI ("" when deliberately skipped)
	PostsByDay         map[string]int                 `json:"postsByDay"`
	SeasonDates        map[string]mlb.SeasonDates     `json:"seasonDates"`
	SeasonDatesFetched string                         `json:"seasonDatesFetched,omitempty"`
	OpeningDays        map[string]OpeningDay          `json:"openingDays"`
	Readings           map[string]astro.RosterReading `json:"readings"` // "season:teamID"
	Adoptions          []Adoption                     `json:"adoptions"`
	Champions          map[string]string              `json:"champions"` // season -> champion
	ChampionDates      map[string]string              `json:"championDates"`
	Picks              []Pick                         `json:"picks"`
	TxnSeen            map[string]string              `json:"txnSeen"` // transaction id -> date
	HotStoveSince      string                         `json:"hotStoveSince,omitempty"`
	HotStove           []HotStoveEntry                `json:"hotStove"`
	Status             Status                         `json:"status"`
	// Rollovers holds the summary of each archived season, keyed by the new season.
	Rollovers map[string]prediction.SeasonSummary `json:"rollovers"`
}

func New() *State {
	st := &State{}
	st.Init()
	return st
}

// Init fills in any nil maps (older files may predate a field).
func (st *State) Init() {
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
		st.OpeningDays = map[string]OpeningDay{}
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

// Path returns the state file's location in dataDir.
func Path(dataDir string) string { return filepath.Join(dataDir, FileName) }

// Read loads the state file (or its .bak) without modifying anything.
// A missing file yields an empty State and an error satisfying os.IsNotExist.
func Read(dataDir string) (st *State, fromBackup bool, err error) {
	data, fromBackup, err := fsutil.ReadFileWithFallback(Path(dataDir), func(b []byte) error {
		var probe State
		return json.Unmarshal(b, &probe)
	})
	if err != nil {
		return New(), false, err
	}
	st = &State{}
	if err := json.Unmarshal(data, st); err != nil {
		return New(), false, err
	}
	st.Init()
	return st, fromBackup, nil
}
