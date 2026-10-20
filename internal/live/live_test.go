package live

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/mlb"
)

func testLogger() *log.Logger { return log.New(io.Discard, "", 0) }

const (
	game5   = 849817 // ALCS Game 5, 2026: Rays 3, Guardians 1
	raysID  = 139
	rootURI = "at://bot/app.bsky.feed.post/pregame"
)

var game5FirstPitch = time.Date(2026, 10, 18, 21, 8, 0, 0, time.UTC)

// fixtureTransport answers Stats API requests from recorded fixtures.
type fixtureTransport struct{}

func (fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	file := map[string]string{
		"/api/v1.1/game/849817/feed/live":    "live_849817_feed.json",
		"/api/v1/game/849817/winProbability": "live_849817_winprob.json",
		"/api/v1/game/849817/content":        "live_849817_content.json",
	}[r.URL.Path]
	data, err := os.ReadFile(filepath.Join("..", "mlb", "testdata", file))
	if file == "" || err != nil {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}, Request: r}, nil
}

// replay serves a finished game as if it were in progress at clock.now:
// only plays that have ended exist, and only clips already published.
type replay struct {
	clock      *clock
	game       *mlb.LiveGame
	plays      []mlb.LivePlay
	highlights []mlb.Highlight
	final      time.Time
	failUntil  time.Time // API errors before this
}

var (
	game5Once       sync.Once
	game5Game       *mlb.LiveGame
	game5Plays      []mlb.LivePlay
	game5Highlights []mlb.Highlight
	game5Err        error
)

// loadReplay parses the fixtures once (the client throttles requests) and
// hands each test its own copy.
func loadReplay(t *testing.T, c *clock) *replay {
	t.Helper()
	game5Once.Do(func() {
		client := mlb.NewClient(&http.Client{Transport: fixtureTransport{}}, testLogger())
		if game5Game, game5Err = client.GetLiveGame(game5); game5Err != nil {
			return
		}
		if game5Plays, game5Err = client.GetLivePlays(game5); game5Err != nil {
			return
		}
		game5Highlights, game5Err = client.GetHighlights(game5)
	})
	if game5Err != nil {
		t.Fatal(game5Err)
	}
	g := *game5Game
	plays := append([]mlb.LivePlay(nil), game5Plays...)
	return &replay{clock: c, game: &g, plays: plays, highlights: game5Highlights, final: plays[len(plays)-1].EndTime.Add(time.Minute)}
}

func (r *replay) GetLiveGame(int) (*mlb.LiveGame, error) {
	if r.clock.now().Before(r.failUntil) {
		return nil, fmt.Errorf("network unreachable")
	}
	g := *r.game
	switch now := r.clock.now(); {
	case now.Before(game5FirstPitch):
		g.Status, g.DetailedState = "Preview", "Pre-Game"
	case now.Before(r.final):
		g.Status, g.DetailedState = "Live", "In Progress"
	}
	return &g, nil
}

func (r *replay) GetLivePlays(int) ([]mlb.LivePlay, error) {
	var out []mlb.LivePlay
	for _, p := range r.plays {
		if !p.EndTime.After(r.clock.now()) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *replay) GetHighlights(int) ([]mlb.Highlight, error) {
	var out []mlb.Highlight
	for _, h := range r.highlights {
		if !h.Date.After(r.clock.now()) {
			out = append(out, h)
		}
	}
	return out, nil
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type sent struct {
	text, parent, root string
	clip               *bluesky.Clip
	image              bool
	uri                string
}

type fakePoster struct {
	mu      sync.Mutex
	posts   []sent
	panicOn int // panic on this post number (1-based), 0 = never
}

func (p *fakePoster) record(s sent) (*bluesky.PostRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.panicOn > 0 && len(p.posts)+1 == p.panicOn {
		p.panicOn = 0
		panic("power cut")
	}
	s.uri = fmt.Sprintf("at://bot/app.bsky.feed.post/%d", len(p.posts)+1)
	p.posts = append(p.posts, s)
	return &bluesky.PostRef{URI: s.uri}, nil
}

func (p *fakePoster) Post(text string, img *bluesky.ImageData) (*bluesky.PostRef, error) {
	return p.record(sent{text: text, image: img != nil})
}
func (p *fakePoster) Reply(text string, img *bluesky.ImageData, parent, root string) (*bluesky.PostRef, error) {
	return p.record(sent{text: text, parent: parent, root: root, image: img != nil})
}
func (p *fakePoster) ReplyClip(text string, clip *bluesky.Clip, parent, root string) (*bluesky.PostRef, error) {
	return p.record(sent{text: text, parent: parent, root: root, clip: clip})
}

func newTestManager(t *testing.T, dataDir string, c *clock, src mlb.LiveProvider, poster bluesky.ClipPoster) *Manager {
	t.Helper()
	m := NewManager(Config{MLB: src, Poster: poster, Logger: testLogger(), DataDir: dataDir})
	m.now, m.sleep = c.now, c.sleep
	return m
}

func registerGame5(m *Manager) {
	m.Register(Thread{
		GamePk: game5, TeamID: raysID, TeamName: "Tampa Bay Rays", Label: "ALCS Game 5",
		FirstPitch: game5FirstPitch, PregameWP: 0.58, RootURI: rootURI,
	})
}

// The whole game, start to finish: every notable play posts once, in order,
// chained reply to reply, with its clip whenever MLB published one in time.
func TestWatch_ReplaysGame5(t *testing.T) {
	c := &clock{t: game5FirstPitch.Add(-5 * time.Minute)}
	src := loadReplay(t, c)
	poster := &fakePoster{}
	m := newTestManager(t, t.TempDir(), c, src, poster)
	registerGame5(m)

	m.watch(game5)

	notable := 0
	for _, p := range src.plays {
		if isNotable(p) {
			notable++
		}
	}
	if notable == 0 || len(poster.posts) != notable {
		t.Fatalf("posted %d updates, want one per notable play (%d)", len(poster.posts), notable)
	}

	parent := rootURI
	clips := 0
	for i, p := range poster.posts {
		if p.root != rootURI || p.parent != parent {
			t.Errorf("post %d: parent %s root %s, want parent %s root %s", i+1, p.parent, p.root, parent, rootURI)
		}
		parent = p.uri
		if n := len([]rune(p.text)); n > 300 {
			t.Errorf("post %d is %d runes:\n%s", i+1, n, p.text)
		}
		if p.clip != nil {
			clips++
		}
	}

	aranda := findPost(poster.posts, "Jonathan Aranda's RBI single")
	if aranda == nil || aranda.clip == nil || !strings.HasSuffix(aranda.clip.VideoURL, ".mp4") {
		t.Errorf("Aranda's single should post with its clip: %+v", aranda)
	} else if !strings.Contains(aranda.text, "🔮 Rays' win chances: science") {
		t.Errorf("missing the cosmic line:\n%s", aranda.text)
	}
	// DeLuca's bases-loaded walk was only clipped hours later: it posts
	// without one after the wait, using MLB's play description.
	if deluca := findPost(poster.posts, "Jonny DeLuca walks"); deluca == nil || deluca.clip != nil {
		t.Errorf("DeLuca's walk should post without a clip: %+v", deluca)
	}
	if clips < 2 {
		t.Errorf("only %d posts had clips", clips)
	}

	th := m.snapshot(game5)
	if !th.Closed || len(th.Pending) != 0 || th.TailURI != parent || th.Posts != notable {
		t.Errorf("thread after the game: closed=%v pending=%d tail=%s posts=%d", th.Closed, len(th.Pending), th.TailURI, th.Posts)
	}
	if len(th.Series) != len(src.plays) {
		t.Errorf("chart has %d points, want %d", len(th.Series), len(src.plays))
	}
	if last := th.Series[len(th.Series)-1]; last.Real != 100 || last.Cosmic != 99 {
		t.Errorf("final point = %+v; the Rays won", last)
	}
	for _, p := range poster.posts {
		t.Logf("\n%s\n[clip: %v]", p.text, p.clip != nil)
	}
}

func findPost(posts []sent, substr string) *sent {
	for i := range posts {
		if strings.Contains(posts[i].text, substr) {
			return &posts[i]
		}
	}
	return nil
}

// A power cut mid-game: the watcher dies mid-post, the bot reboots with the
// state file, and the new watcher finishes the thread with nothing posted
// twice and nothing lost.
func TestWatch_SurvivesCrashAndRestart(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t: game5FirstPitch.Add(-5 * time.Minute)}
	src := loadReplay(t, c)

	poster := &fakePoster{panicOn: 3}
	m1 := newTestManager(t, dir, c, src, poster)
	registerGame5(m1)
	m1.mu.Lock()
	m1.running[game5] = true
	m1.mu.Unlock()
	m1.runWatcher(game5) // recovers the panic
	if len(poster.posts) != 2 {
		t.Fatalf("posted %d before the crash, want 2", len(poster.posts))
	}
	if m1.running[game5] {
		t.Error("a dead watcher must not count as running")
	}

	// Reboot: a fresh manager from disk, back online 2 minutes later.
	c.sleep(2 * time.Minute)
	m2 := newTestManager(t, dir, c, src, poster)
	th := m2.snapshot(game5)
	if th == nil || th.Posts != 2 || th.TailURI != poster.posts[1].uri {
		t.Fatalf("restored thread = %+v", th)
	}
	m2.watch(game5)

	seen := map[string]bool{}
	for i, p := range poster.posts {
		if seen[p.text] {
			t.Errorf("post %d is a duplicate:\n%s", i+1, p.text)
		}
		seen[p.text] = true
		if i > 0 && p.parent != poster.posts[i-1].uri {
			t.Errorf("post %d broke the chain", i+1)
		}
	}
	notable := 0
	for _, p := range src.plays {
		if isNotable(p) {
			notable++
		}
	}
	if len(poster.posts) != notable {
		t.Errorf("posted %d total, want %d", len(poster.posts), notable)
	}
}

// Back online after a long outage: plays from while we were gone are
// skipped, not dumped into the thread all at once.
func TestWatch_SkipsPlaysMissedWhileOffline(t *testing.T) {
	c := &clock{t: game5FirstPitch.Add(-5 * time.Minute)}
	src := loadReplay(t, c)
	poster := &fakePoster{}
	m := newTestManager(t, t.TempDir(), c, src, poster)
	registerGame5(m)

	// Offline from first pitch until the 6th inning.
	var sixth time.Time
	for _, p := range src.plays {
		if p.Inning == 6 {
			sixth = p.EndTime
			break
		}
	}
	c.t = sixth
	m.watch(game5)

	for _, p := range poster.posts {
		if strings.Contains(p.text, "Top 1 ") || strings.Contains(p.text, "Top 2 ") || strings.Contains(p.text, "Bot 2 ") {
			t.Errorf("posted a stale early-inning play:\n%s", p.text)
		}
	}
	if len(poster.posts) == 0 {
		t.Error("expected late-game updates")
	}
	if th := m.snapshot(game5); len(th.Series) != len(src.plays) {
		t.Errorf("the chart should still cover the whole game: %d points", len(th.Series))
	}
}

// Network trouble: the watcher backs off and keeps going instead of dying.
func TestWatch_RetriesThroughOutage(t *testing.T) {
	c := &clock{t: game5FirstPitch}
	src := loadReplay(t, c)
	src.failUntil = game5FirstPitch.Add(5 * time.Minute)
	poster := &fakePoster{}
	m := newTestManager(t, "", c, src, poster)
	registerGame5(m)
	m.watch(game5)
	if th := m.snapshot(game5); !th.Closed || th.Posts == 0 {
		t.Errorf("thread = closed %v, %d posts", th.Closed, th.Posts)
	}
}

func TestWatch_PostponedClosesThread(t *testing.T) {
	c := &clock{t: game5FirstPitch}
	src := loadReplay(t, c)
	src.game.DetailedState = "Postponed"
	src.final = game5FirstPitch.Add(-time.Hour) // status passes through as-is
	m := newTestManager(t, "", c, src, &fakePoster{})
	registerGame5(m)
	m.watch(game5)
	if th := m.snapshot(game5); !th.Closed || th.Posts != 0 {
		t.Errorf("thread = %+v", th)
	}
}

func TestWrapUp(t *testing.T) {
	c := &clock{t: game5FirstPitch.Add(-5 * time.Minute)}
	src := loadReplay(t, c)
	m := newTestManager(t, "", c, src, &fakePoster{})

	if _, ready := m.WrapUp(123); !ready {
		t.Error("an untracked game is always ready")
	}
	var nilM *Manager
	if _, ready := nilM.WrapUp(game5); !ready {
		t.Error("a nil manager is always ready")
	}

	registerGame5(m)
	m.supervising = true
	if _, ready := m.WrapUp(game5); ready {
		t.Error("not ready while the watcher is live")
	}

	m.watch(game5)
	w, ready := m.WrapUp(game5)
	if !ready || w.ParentURI == "" || w.Chart == nil || len(w.Chart.Bytes) == 0 {
		t.Fatalf("wrap-up = %+v ready=%v", w, ready)
	}
	if !strings.Contains(w.Chart.Alt, "Rays") {
		t.Errorf("alt = %s", w.Chart.Alt)
	}
	if os.Getenv("SAVE_CHART") != "" {
		os.WriteFile(os.Getenv("SAVE_CHART"), w.Chart.Bytes, 0644)
	}

	m.Finish(game5)
	if th := m.snapshot(game5); !th.WrappedUp {
		t.Error("Finish should mark the thread wrapped up")
	}
}

// A watcher still waiting long after first pitch doesn't hold the result forever.
func TestWrapUp_GivesUpWaiting(t *testing.T) {
	c := &clock{t: game5FirstPitch}
	m := newTestManager(t, "", c, nil, &fakePoster{})
	registerGame5(m)
	m.supervising = true
	c.sleep(wrapUpGrace + time.Minute)
	if _, ready := m.WrapUp(game5); !ready {
		t.Error("should be ready after the grace period")
	}
}

func TestSupervise(t *testing.T) {
	c := &clock{t: game5FirstPitch.Add(-time.Hour)}
	block := make(chan struct{})
	src := &blockingSource{block: block}
	m := newTestManager(t, "", c, src, &fakePoster{})
	registerGame5(m)

	m.supervise()
	if m.running[game5] {
		t.Fatal("too early to watch")
	}

	c.t = game5FirstPitch.Add(-5 * time.Minute)
	m.supervise()
	m.supervise() // no second watcher for the same game
	m.mu.Lock()
	running := m.running[game5]
	m.mu.Unlock()
	if !running {
		t.Fatal("watcher should be running")
	}
	if n := src.calls(); n > 1 {
		t.Errorf("%d watchers started", n)
	}
	close(block)

	// Closed and old: pruned from the file.
	m.update(game5, func(t *Thread) { t.Closed = true })
	c.t = game5FirstPitch.Add(pruneAfter + time.Hour)
	m.supervise()
	if m.snapshot(game5) != nil {
		t.Error("old closed thread should be pruned")
	}
}

// blockingSource holds the watcher in its first API call.
type blockingSource struct {
	mu    sync.Mutex
	n     int
	block chan struct{}
}

func (b *blockingSource) calls() int { b.mu.Lock(); defer b.mu.Unlock(); return b.n }
func (b *blockingSource) GetLiveGame(int) (*mlb.LiveGame, error) {
	b.mu.Lock()
	b.n++
	b.mu.Unlock()
	<-b.block
	return nil, fmt.Errorf("stopped")
}
func (b *blockingSource) GetLivePlays(int) ([]mlb.LivePlay, error)   { return nil, nil }
func (b *blockingSource) GetHighlights(int) ([]mlb.Highlight, error) { return nil, nil }

func TestRegister_IsIdempotentAndSeedsOffset(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t: game5FirstPitch}
	m := newTestManager(t, dir, c, nil, &fakePoster{})
	registerGame5(m)
	m.update(game5, func(t *Thread) { t.Posts = 4 })
	registerGame5(m)

	m2 := newTestManager(t, dir, c, nil, &fakePoster{})
	th := m2.snapshot(game5)
	if th == nil || th.Posts != 4 || th.LastAtBat != -1 {
		t.Fatalf("thread = %+v", th)
	}
	if th.Offset < 7.9 || th.Offset > 8.1 { // 58% - 50%
		t.Errorf("a 58%% pregame call should seed +8, got %.1f", th.Offset)
	}
}
