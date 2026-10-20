package live

import (
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/image"
	"github.com/tlugger/rockiscope/internal/mlb"
)

const (
	// superviseEvery is how often the supervisor makes sure every active game
	// has a watcher. It's the recovery path after a crash or reboot.
	superviseEvery = 2 * time.Minute
	// watchFrom/watchUntil bound when a game is worth watching, relative to
	// first pitch. Past watchUntil (a marathon plus a rain delay) we give up.
	watchFrom  = 10 * time.Minute
	watchUntil = 10 * time.Hour
	// wrapUpGrace is how long after first pitch the scheduler will wait on a
	// watcher before posting the result anyway.
	wrapUpGrace = 7 * time.Hour
	// pruneAfter drops finished threads from the state file.
	pruneAfter = 72 * time.Hour
)

// Config wires a Manager.
type Config struct {
	MLB     mlb.LiveProvider
	Poster  bluesky.ClipPoster
	Logger  *log.Logger
	DataDir string
}

// Manager owns every live thread: it persists them, keeps a watcher running
// for each game in progress, and tells the scheduler when a game's thread is
// ready for its result post. All methods are safe to call from the
// scheduler's goroutine while watchers run; a nil *Manager is a no-op.
type Manager struct {
	mlb     mlb.LiveProvider
	poster  bluesky.ClipPoster
	logger  *log.Logger
	dataDir string
	now     func() time.Time
	sleep   func(time.Duration)

	mu          sync.Mutex
	threads     map[int]*Thread
	running     map[int]bool
	supervising bool // the supervisor loop is running
}

func NewManager(cfg Config) *Manager {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	m := &Manager{
		mlb:     cfg.MLB,
		poster:  cfg.Poster,
		logger:  cfg.Logger,
		dataDir: cfg.DataDir,
		now:     time.Now,
		sleep:   time.Sleep,
		running: map[int]bool{},
	}
	threads, err := loadThreads(cfg.DataDir)
	if err != nil {
		m.logger.Printf("warning: live threads unreadable, starting fresh: %v", err)
	}
	m.threads = threads
	return m
}

// Register starts tracking a game whose pregame post is up. Registering the
// same game again is a no-op, so callers can re-register on every tick.
func (m *Manager) Register(t Thread) {
	if m == nil || t.GamePk == 0 || t.RootURI == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.threads[t.GamePk]; ok {
		return
	}
	t.LastAtBat = -1
	t.Offset = initialOffset(t.PregameWP)
	m.threads[t.GamePk] = &t
	m.saveLocked()
	m.logger.Printf("live thread registered for game %d (%s)", t.GamePk, t.TeamName)
}

// WrapUp is what the result post needs from the live thread.
type WrapUp struct {
	ParentURI string // reply here to continue the chain ("" = the pregame post)
	Chart     *bluesky.ImageData
}

// WrapUp reports whether gamePk's result can be posted now, and what to
// attach. It's not ready while a watcher is still posting the final plays.
func (m *Manager) WrapUp(gamePk int) (WrapUp, bool) {
	if m == nil {
		return WrapUp{}, true
	}
	m.mu.Lock()
	t, ok := m.threads[gamePk]
	if !ok {
		m.mu.Unlock()
		return WrapUp{}, true
	}
	t = t.clone()
	ready := t.Closed || !m.supervising || m.now().After(t.FirstPitch.Add(wrapUpGrace))
	m.mu.Unlock()
	if !ready {
		return WrapUp{}, false
	}
	return WrapUp{ParentURI: t.TailURI, Chart: m.chart(t)}, true
}

// Finish marks gamePk's result as posted; its watcher never restarts.
func (m *Manager) Finish(gamePk int) {
	if m == nil {
		return
	}
	m.update(gamePk, func(t *Thread) { t.WrappedUp, t.Closed = true, true })
}

func (m *Manager) chart(t *Thread) *bluesky.ImageData {
	if len(t.Series) < 2 {
		return nil
	}
	title := fmt.Sprintf("%s @ %s", t.AwayName, t.HomeName)
	if t.Label != "" {
		title += " · " + t.Label
	}
	png, w, h, err := image.WinProbChart(title, t.Series)
	if err != nil {
		m.logger.Printf("warning: win probability chart: %v", err)
		return nil
	}
	last := t.Series[len(t.Series)-1]
	return &bluesky.ImageData{
		Bytes: png, Width: w, Height: h,
		Alt: fmt.Sprintf("Chart of the %s' win probability play by play, MLB's model vs the stars' cosmic correction. Final reading: science %.0f%%, stars %.0f%%.",
			formatter.Nickname(t.TeamName), last.Real, last.Cosmic),
	}
}

// Run is the supervisor loop. It never returns.
func (m *Manager) Run() {
	m.mu.Lock()
	m.supervising = true
	m.mu.Unlock()
	m.logger.Println("live game supervisor started")
	for {
		m.supervise()
		m.sleep(superviseEvery)
	}
}

// supervise starts a watcher for every game in its window that lacks one.
func (m *Manager) supervise() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	pruned := false
	for pk, t := range m.threads {
		if t.Closed && now.Sub(t.FirstPitch) > pruneAfter {
			delete(m.threads, pk)
			pruned = true
			continue
		}
		if t.Closed || m.running[pk] {
			continue
		}
		if now.Before(t.FirstPitch.Add(-watchFrom)) {
			continue
		}
		if now.After(t.FirstPitch.Add(watchUntil)) {
			m.logger.Printf("game %d ran past the watch window, closing its live thread", pk)
			t.Closed = true
			pruned = true
			continue
		}
		m.running[pk] = true
		go m.runWatcher(pk)
	}
	if pruned {
		m.saveLocked()
	}
}

// runWatcher runs one game's watcher, surviving its panics. The supervisor
// starts a fresh one on its next pass if the game isn't over.
func (m *Manager) runWatcher(gamePk int) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Printf("PANIC in live watcher for game %d (restarting soon): %v\n%s", gamePk, r, debug.Stack())
		}
		m.mu.Lock()
		delete(m.running, gamePk)
		m.mu.Unlock()
	}()
	m.logger.Printf("live watcher started for game %d", gamePk)
	m.watch(gamePk)
	m.logger.Printf("live watcher finished for game %d", gamePk)
}

// snapshot copies a thread for use outside the lock.
func (m *Manager) snapshot(gamePk int) *Thread {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.threads[gamePk]
	if !ok {
		return nil
	}
	return t.clone()
}

// update mutates a thread under the lock and persists it.
func (m *Manager) update(gamePk int, fn func(*Thread)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.threads[gamePk]
	if !ok {
		return
	}
	fn(t)
	m.saveLocked()
}

func (m *Manager) saveLocked() {
	if err := saveThreads(m.dataDir, m.threads); err != nil {
		m.logger.Printf("warning: saving live threads: %v", err)
	}
}
