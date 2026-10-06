package scheduler

import (
	"errors"
	"fmt"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/mlb"
)

const (
	// retryDelay is how soon to try again after something fails.
	retryDelay = 15 * time.Minute
	// maxSeasonalSleep bounds any single sleep so a bad wake calculation can
	// never park the bot for days.
	maxSeasonalSleep = 6 * time.Hour
	// maxPostsPerDay is a runaway guard for the seasonal modes. A real day
	// tops out around 8 (report card thread + adoption + games + replies).
	maxPostsPerDay = 12
	// Chatter (anything not tied to a game) only goes out between these hours.
	chatterStartHour = 8
	chatterEndHour   = 22
	dailyCheckHour   = 9
)

var errDailyCap = errors.New("daily post cap reached")

// tickResult collects when to wake next and what failed during one pass.
type tickResult struct {
	now  time.Time
	next time.Time
	errs []error
}

func (r *tickResult) wakeAt(t time.Time) {
	if !t.After(r.now) {
		return
	}
	if r.next.IsZero() || t.Before(r.next) {
		r.next = t
	}
}

func (r *tickResult) fail(task string, err error) {
	r.errs = append(r.errs, fmt.Errorf("%s: %w", task, err))
}

func (r *tickResult) err() error { return errors.Join(r.errs...) }

// seasonalTick runs every postseason/offseason/spring task and returns how long to sleep.
func (s *Scheduler) seasonalTick(p PhaseInfo) time.Duration {
	r := s.runSeasonalTasks(p)
	retry := false
	for _, err := range r.errs {
		if errors.Is(err, errDailyCap) {
			s.logger.Printf("daily post cap (%d) reached, holding the rest until tomorrow", maxPostsPerDay)
			continue
		}
		s.logger.Printf("warning: %v", err)
		retry = true
	}

	d := r.next.Sub(r.now)
	if retry && d > retryDelay {
		d = retryDelay
	}
	if d > maxSeasonalSleep {
		d = maxSeasonalSleep
	}
	if d < time.Minute {
		d = time.Minute
	}
	return d
}

func (s *Scheduler) runSeasonalTasks(p PhaseInfo) *tickResult {
	now := s.denverNow()
	r := &tickResult{now: now}
	r.wakeAt(s.nextClock(now, dailyCheckHour, 0))

	switch p.Phase {
	case PhasePostseason:
		// Settle any regular-season stragglers before closing the books.
		if err := s.checkForCompletedGames(); err != nil {
			s.logger.Printf("warning: could not check completed games: %v", err)
		}
		s.reportCardTask(p.Season, r)
		s.postseasonTask(p.Season, r)
	case PhaseOffseason:
		s.reportCardTask(p.Season, r)
		if s.postseasonNeedsWrapUp(p.Season) {
			s.postseasonTask(p.Season, r)
		}
		s.hotStoveTask(r)
		s.countdownTask(p, r)
	case PhaseSpring:
		s.rolloverTask(p.Season, r)
		s.countdownTask(p, r)
		s.springTask(p.Season, r)
	}
	return r
}

// postOnce publishes text exactly once per idempotency key, as a top-level post
// or (with parentURI) a reply. The key and resulting URI are persisted
// immediately, so a crash right after posting can't cause a duplicate.
func (s *Scheduler) postOnce(key, text string, img *bluesky.ImageData, parentURI, rootURI string) (string, error) {
	st := s.state()
	if uri, ok := st.Done[key]; ok {
		return uri, nil
	}
	today := s.today()
	if st.PostsByDay[today] >= maxPostsPerDay {
		return "", errDailyCap
	}

	text = formatter.Fit(text, formatter.MaxPostLength)
	if s.dryRun {
		s.logger.Printf("would post [%s]", key)
	} else {
		s.logger.Printf("posting [%s]:\n%s", key, text)
	}

	var ref *bluesky.PostRef
	var err error
	if parentURI != "" {
		if rootURI == "" {
			rootURI = parentURI
		}
		ref, err = s.poster.Reply(text, img, parentURI, rootURI)
	} else {
		ref, err = s.poster.Post(text, img)
	}
	if err != nil {
		return "", fmt.Errorf("posting %s: %w", key, err)
	}

	st.Done[key] = ref.URI
	st.PostsByDay[today]++
	s.saveSeasonState()
	return ref.URI, nil
}

// chatterAllowed reports whether non-game posts may go out now; if not, it
// schedules a wake for the start of the window.
func (s *Scheduler) chatterAllowed(r *tickResult) bool {
	if s.ignoreTimeGates {
		return true
	}
	h := r.now.Hour()
	if h >= chatterStartHour && h < chatterEndHour {
		return true
	}
	r.wakeAt(s.nextClock(r.now, chatterStartHour, 0))
	return false
}

// pregameWindow decides whether a pregame post for a game at firstPitch is due
// now (an hour before first pitch). Too early schedules a wake; once the game
// is underway the window is gone. Predicting after first pitch would be cheating.
func (s *Scheduler) pregameWindow(firstPitch time.Time, r *tickResult) (due, missed bool) {
	if s.ignoreTimeGates {
		return true, false
	}
	postAt := firstPitch.Add(-time.Hour)
	if r.now.Before(postAt) {
		r.wakeAt(postAt)
		return false, false
	}
	if r.now.After(firstPitch.Add(15 * time.Minute)) {
		return false, true
	}
	return true, false
}

// pollForResult schedules the next look at a game that isn't final yet.
func (s *Scheduler) pollForResult(firstPitch time.Time, r *tickResult) {
	if firstPitch.IsZero() || r.now.After(firstPitch.Add(2*time.Hour)) {
		r.wakeAt(r.now.Add(20 * time.Minute))
		return
	}
	r.wakeAt(firstPitch.Add(150 * time.Minute))
}

// nextClock returns the next occurrence of hour:min in Denver after now.
func (s *Scheduler) nextClock(now time.Time, hour, min int) time.Time {
	now = now.In(mlb.DenverLocation())
	t := clockOn(now, hour, min)
	if !t.After(now) {
		t = clockOn(now.AddDate(0, 0, 1), hour, min)
	}
	return t
}

func clockOn(day time.Time, hour, min int) time.Time {
	loc := mlb.DenverLocation()
	day = day.In(loc)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, min, 0, 0, loc)
}

// daysBetween counts calendar days from a to b ("2006-01-02"), negative if b is earlier.
func daysBetween(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

func (s *Scheduler) picksRecord(kind string, season int) string {
	correct, total := 0, 0
	for _, p := range s.state().Picks {
		if p.Kind != kind || p.Season != season || (p.Result != "W" && p.Result != "L") {
			continue
		}
		total++
		if p.Result == p.Pick {
			correct++
		}
	}
	return fmt.Sprintf("%d/%d", correct, total)
}

func (s *Scheduler) findPick(kind string, gamePk int) *pick {
	st := s.state()
	for i := range st.Picks {
		if st.Picks[i].Kind == kind && st.Picks[i].GamePk == gamePk {
			return &st.Picks[i]
		}
	}
	return nil
}

func (s *Scheduler) pitcherLine(info *mlb.PitcherInfo) (string, *mlb.PitcherStats) {
	if info == nil {
		return "", nil
	}
	stats, err := s.mlb.GetPitcherStats(info.ID)
	if err != nil || stats == nil {
		if err != nil {
			s.logger.Printf("warning: pitcher stats for %s: %v", info.FullName, err)
		}
		return info.FullName, nil
	}
	if stats.InningsPitched > 0 {
		return fmt.Sprintf("%s (%.2f ERA)", info.FullName, stats.ERA), stats
	}
	return info.FullName, stats
}

func formatClock(t time.Time) string {
	t = t.In(mlb.DenverLocation())
	zone, _ := t.Zone()
	return t.Format("3:04 PM ") + zone
}
