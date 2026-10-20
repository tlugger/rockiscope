package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/live"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// fakeLive stands in for live.Manager.
type fakeLive struct {
	registered map[int]live.Thread
	ready      bool
	tail       string
	finished   []int
}

func newFakeLive() *fakeLive { return &fakeLive{registered: map[int]live.Thread{}, ready: true} }

func (f *fakeLive) Register(t live.Thread) {
	if _, ok := f.registered[t.GamePk]; !ok {
		f.registered[t.GamePk] = t
	}
}

func (f *fakeLive) WrapUp(int) (live.WrapUp, bool) {
	if !f.ready {
		return live.WrapUp{}, false
	}
	return live.WrapUp{ParentURI: f.tail, Chart: &bluesky.ImageData{Bytes: []byte("png"), Alt: "chart"}}, true
}

func (f *fakeLive) Finish(gamePk int) { f.finished = append(f.finished, gamePk) }

func TestPostseason_LiveThread(t *testing.T) {
	h := postseasonHarness(t)
	h.now = denverAt(2026, 10, 6, 18, 31)
	s := h.scheduler()
	fl := newFakeLive()
	s.live = fl
	p := PhaseInfo{Phase: PhasePostseason, Season: 2026}

	// The pregame post registers the game for live updates.
	s.runSeasonalTasks(p)
	pick := s.findPick(pickPostseason, 3)
	if pick == nil {
		t.Fatal("no pick")
	}
	th, ok := fl.registered[3]
	if !ok || th.RootURI != pick.PostURI || th.TeamID != 135 || th.Label != "D Game 3" || th.PregameWP != pick.WinProbability {
		t.Fatalf("registered = %+v (pick %+v)", th, pick)
	}

	// Game over, but the watcher is still posting the last play: hold the result.
	h.season.games[2] = seriesGame(3, "D", 3, 5, denverAt(2026, 10, 6, 19, 30), brewers, padres, 135)
	h.now = denverAt(2026, 10, 6, 22, 40)
	fl.ready = false
	r := s.runSeasonalTasks(p)
	if len(h.poster.find("Postseason picks: ")) != 0 {
		t.Fatal("posted the result before the live thread finished")
	}
	if pick.Result != "" {
		t.Error("pick settled early")
	}
	if want := h.now.Add(2 * time.Minute); !r.next.Equal(want) {
		t.Errorf("next wake %s, want %s", r.next, want)
	}

	// Thread done: the result continues the chain with the chart attached.
	fl.ready, fl.tail = true, "at://test/live-9"
	h.now = h.now.Add(2 * time.Minute)
	s.runSeasonalTasks(p)
	res := h.poster.find("Postseason picks: ")
	if len(res) != 1 || res[0].parent != "at://test/live-9" || res[0].root != pick.PostURI || !res[0].image {
		t.Fatalf("result = %+v", res)
	}
	if len(fl.finished) != 1 || fl.finished[0] != 3 {
		t.Errorf("finished = %v", fl.finished)
	}
}

func TestRegularSeason_LiveThread(t *testing.T) {
	denver := mlb.DenverLocation()
	gameTime := time.Date(2026, 4, 8, 19, 10, 0, 0, denver)
	poster := &recordingPoster{}
	fl := newFakeLive()
	game := &mlb.Game{
		GamePk: 1234, GameDateTime: gameTime, OfficialDate: "2026-04-08", Status: "Live",
		IsHome: true, HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"}, AwayTeam: mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
	}
	now := gameTime.Add(30 * time.Minute)
	s := &Scheduler{
		mlb: &mockMLB{game: game, gamesSince: []mlb.GameResult{
			{GamePk: 1234, Date: "2026-04-08", Opponent: "Houston Astros", Won: true, RockiesScore: 4, OppScore: 2},
		}},
		poster: poster,
		now:    func() time.Time { return now },
		sleep:  func(time.Duration) {},
		logger: testLogger(),
		live:   fl,
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{
				{Date: "2026-04-08", Opponent: "Houston Astros", Predicted: "W", GamePK: 1234, WinProbability: 0.61, PostURI: "at://test/pregame"},
			},
			Current: prediction.DefaultWeights(),
		},
		lastPostDate: "2026-04-08",
	}

	// Already posted today (e.g. a restart mid-game): the tick registers it.
	if err := s.tick(); err != nil {
		t.Fatal(err)
	}
	th, ok := fl.registered[1234]
	if !ok || th.RootURI != "at://test/pregame" || th.TeamID != mlb.RockiesID || th.TeamName != "Colorado Rockies" || th.PregameWP != 0.61 {
		t.Fatalf("registered = %+v", th)
	}

	// Final, watcher still busy: no follow-up yet.
	fl.ready = false
	if err := s.checkForCompletedGames(); err != nil {
		t.Fatal(err)
	}
	if len(poster.sent) != 0 || s.predHistory.Predictions[0].Actual != "" {
		t.Fatalf("followed up too early: %+v", poster.sent)
	}

	fl.ready, fl.tail = true, "at://test/live-4"
	if err := s.checkForCompletedGames(); err != nil {
		t.Fatal(err)
	}
	if len(poster.sent) != 1 {
		t.Fatalf("sent = %+v", poster.sent)
	}
	got := poster.sent[0]
	if got.parent != "at://test/live-4" || got.root != "at://test/pregame" || !got.image || !strings.Contains(got.text, "Rockies W") {
		t.Errorf("follow-up = %+v", got)
	}
	if len(fl.finished) != 1 {
		t.Errorf("finished = %v", fl.finished)
	}
}

// Without a live thread the result still replies straight to the pregame post.
func TestRegularSeason_NoLiveThreadRepliesToPregame(t *testing.T) {
	poster := &recordingPoster{}
	s := &Scheduler{
		mlb:    &mockMLB{gamesSince: []mlb.GameResult{{GamePk: 1234, Date: "2026-04-08", Won: false}}},
		poster: poster,
		now:    func() time.Time { return time.Date(2026, 4, 8, 23, 0, 0, 0, time.UTC) },
		sleep:  func(time.Duration) {},
		logger: testLogger(),
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{{Date: "2026-04-08", Predicted: "L", GamePK: 1234, PostURI: "at://test/pregame"}},
			Current:     prediction.DefaultWeights(),
		},
	}
	if err := s.checkForCompletedGames(); err != nil {
		t.Fatal(err)
	}
	if len(poster.sent) != 1 || poster.sent[0].parent != "at://test/pregame" || poster.sent[0].root != "at://test/pregame" || poster.sent[0].image {
		t.Errorf("sent = %+v", poster.sent)
	}
}
