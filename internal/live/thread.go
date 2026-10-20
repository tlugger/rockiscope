package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/fsutil"
	"github.com/tlugger/rockiscope/internal/image"
)

const FileName = "live_threads.json"

// Thread is one game's live thread. It's persisted after every change, so a
// watcher restarted after a power cut picks up exactly where the last one
// stopped: same tail post, same cosmic offset, same queue of plays awaiting
// their clips.
type Thread struct {
	GamePk     int       `json:"gamePk"`
	TeamID     int       `json:"teamId"`
	TeamName   string    `json:"teamName"`
	Label      string    `json:"label,omitempty"` // "ALCS Game 5", for the chart
	FirstPitch time.Time `json:"firstPitch"`
	PregameWP  float64   `json:"pregameWp"` // 0-1, the pregame post's call
	RootURI    string    `json:"rootUri"`   // the pregame post
	TailURI    string    `json:"tailUri,omitempty"`

	// LastAtBat is the highest completed atBatIndex already looked at.
	LastAtBat int             `json:"lastAtBat"`
	Offset    float64         `json:"offset"` // the stars' running correction, points
	Pending   []Pending       `json:"pending,omitempty"`
	Series    []image.WPPoint `json:"series,omitempty"`
	Posts     int             `json:"posts"`
	AwayName  string          `json:"awayName,omitempty"`
	HomeName  string          `json:"homeName,omitempty"`

	// Closed means the watcher is done (game over and queue drained, or the
	// game was postponed). WrappedUp means the scheduler posted the result.
	Closed    bool `json:"closed,omitempty"`
	WrappedUp bool `json:"wrappedUp,omitempty"`
}

// Pending is a notable play waiting for its highlight clip.
type Pending struct {
	AtBatIndex int                  `json:"atBatIndex"`
	PlayIDs    []string             `json:"playIds"`
	BatterID   int                  `json:"batterId"`
	EndTime    time.Time            `json:"endTime"`
	Deadline   time.Time            `json:"deadline"` // post without a clip after this
	Update     formatter.LiveUpdate `json:"update"`
	Fallback   string               `json:"fallback"` // headline when there's no clip
	Failures   int                  `json:"failures,omitempty"`
}

// tail is the post the next update replies to.
func (t *Thread) tail() string {
	if t.TailURI != "" {
		return t.TailURI
	}
	return t.RootURI
}

func (t *Thread) clone() *Thread {
	c := *t
	c.Pending = append([]Pending(nil), t.Pending...)
	c.Series = append([]image.WPPoint(nil), t.Series...)
	return &c
}

func loadThreads(dataDir string) (map[int]*Thread, error) {
	threads := map[int]*Thread{}
	if dataDir == "" {
		return threads, nil
	}
	data, _, err := fsutil.ReadFileWithFallback(filepath.Join(dataDir, FileName), func(b []byte) error {
		var probe map[int]*Thread
		return json.Unmarshal(b, &probe)
	})
	if os.IsNotExist(err) {
		return threads, nil
	}
	if err != nil {
		return threads, err
	}
	if err := json.Unmarshal(data, &threads); err != nil {
		return map[int]*Thread{}, err
	}
	return threads, nil
}

func saveThreads(dataDir string, threads map[int]*Thread) error {
	if dataDir == "" {
		return nil
	}
	data, err := json.MarshalIndent(threads, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(filepath.Join(dataDir, FileName), data, 0644)
}
