package scheduler

import (
	"fmt"
	"time"

	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/seasonstate"
)

// Phase is where we are in the baseball calendar.
type Phase string

const (
	PhaseRegular    Phase = "regular-season"
	PhasePostseason Phase = "postseason"
	PhaseOffseason  Phase = "offseason"
	PhaseSpring     Phase = "spring-training"
)

// PhaseInfo describes the current phase.
type PhaseInfo struct {
	Phase Phase
	// Season is the baseball season this phase belongs to. In January 2027 the
	// offseason still belongs to 2026; spring 2027 belongs to 2027.
	Season int
	// Upcoming is the next season's calendar (for the Opening Day countdown).
	Upcoming *mlb.SeasonDates
	// Source is "api", "cache" or "fallback" (no calendar data at all).
	Source string
}

func (p PhaseInfo) String() string {
	return fmt.Sprintf("%s (season %d, via %s)", p.Phase, p.Season, p.Source)
}

// classifyPhase is the pure phase decision for a Denver calendar date.
// rockiesOpener, when known, replaces MLB's regular-season start so that
// international openers don't end spring early for the Rockies.
func classifyPhase(today string, year int, thisYear, nextYear *mlb.SeasonDates, rockiesOpener string, championCrowned bool) PhaseInfo {
	regularStart := thisYear.RegularStart
	if rockiesOpener != "" && rockiesOpener > regularStart {
		regularStart = rockiesOpener
	}
	switch {
	case thisYear.SpringStart != "" && today < thisYear.SpringStart:
		return PhaseInfo{Phase: PhaseOffseason, Season: year - 1, Upcoming: thisYear}
	case today < regularStart:
		return PhaseInfo{Phase: PhaseSpring, Season: year, Upcoming: thisYear}
	case today <= thisYear.RegularEnd:
		return PhaseInfo{Phase: PhaseRegular, Season: year}
	case today <= thisYear.PostEnd && !championCrowned:
		return PhaseInfo{Phase: PhasePostseason, Season: year, Upcoming: nextYear}
	default:
		return PhaseInfo{Phase: PhaseOffseason, Season: year, Upcoming: nextYear}
	}
}

// recordStatus saves the phase for the dashboard, only when it changes.
func (s *Scheduler) recordStatus(p PhaseInfo) {
	if s.season == nil {
		return
	}
	st := s.state()
	if st.Status.Phase == string(p.Phase) && st.Status.Season == p.Season {
		return
	}
	st.Status = seasonstate.Status{Phase: string(p.Phase), Season: p.Season, UpdatedAt: s.now().UTC().Format(time.RFC3339)}
	s.saveSeasonState()
}

// fallbackPhase guesses from the month when no calendar data is available at all.
func fallbackPhase(now time.Time) PhaseInfo {
	y := now.Year()
	md := int(now.Month())*100 + now.Day()
	switch {
	case md < 218:
		return PhaseInfo{Phase: PhaseOffseason, Season: y - 1, Source: "fallback"}
	case md < 325:
		return PhaseInfo{Phase: PhaseSpring, Season: y, Source: "fallback"}
	case md < 930:
		return PhaseInfo{Phase: PhaseRegular, Season: y, Source: "fallback"}
	case md < 1101:
		return PhaseInfo{Phase: PhasePostseason, Season: y, Source: "fallback"}
	default:
		return PhaseInfo{Phase: PhaseOffseason, Season: y, Source: "fallback"}
	}
}

// CurrentPhase works out the phase from MLB's season calendar, refreshing the
// cached calendar at most once a day. API failures fall back to the cache,
// then to a month-based guess, so a flaky network never stops the bot.
func (s *Scheduler) CurrentPhase() PhaseInfo {
	now := s.denverNow()
	if s.season == nil {
		return PhaseInfo{Phase: PhaseRegular, Season: now.Year(), Source: "legacy"}
	}
	st := s.state()
	today := now.Format("2006-01-02")
	year := now.Year()

	source := "cache"
	if st.SeasonDatesFetched != today {
		fetched := true
		for _, y := range []int{year, year + 1} {
			d, err := s.season.GetSeasonDates(y)
			if err != nil {
				// Next year's calendar is often unpublished; that's fine.
				if y == year {
					s.logger.Printf("warning: season calendar unavailable, using cache: %v", err)
					fetched = false
				}
				continue
			}
			st.SeasonDates[seasonKey(y)] = *d
		}
		if fetched {
			st.SeasonDatesFetched = today
			source = "api"
			s.saveSeasonState()
		}
	}

	thisYear, ok := st.SeasonDates[seasonKey(year)]
	if !ok {
		return fallbackPhase(now)
	}
	var nextYear *mlb.SeasonDates
	if d, ok := st.SeasonDates[seasonKey(year+1)]; ok {
		nextYear = &d
	}
	opener := st.OpeningDays[seasonKey(year)].Date
	_, crowned := st.Champions[seasonKey(year)]

	p := classifyPhase(today, year, &thisYear, nextYear, opener, crowned)
	p.Source = source
	return p
}
