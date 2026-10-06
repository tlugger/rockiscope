package scheduler

import (
	"sort"
	"time"

	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/mlb"
)

const countdownHour = 10

// openingDayFor finds the Rockies' first regular-season game, caching it once found.
func (s *Scheduler) openingDayFor(dates *mlb.SeasonDates) openingDay {
	st := s.state()
	if od, ok := st.OpeningDays[dates.Season]; ok {
		return od
	}
	fallback := openingDay{Date: dates.RegularStart}
	start, err := time.Parse("2006-01-02", dates.RegularStart)
	if err != nil {
		return fallback
	}
	games, err := s.season.GetRockiesGamesBetween(dates.RegularStart, start.AddDate(0, 0, 21).Format("2006-01-02"), "R")
	if err != nil || len(games) == 0 {
		if err != nil {
			s.logger.Printf("warning: Opening Day lookup failed, using league start: %v", err)
		}
		return fallback
	}
	sort.Slice(games, func(i, j int) bool { return games[i].GameDateTime.Before(games[j].GameDateTime) })
	g := games[0]
	od := openingDay{Date: g.OfficialDate, Opponent: g.Opponent().Name, IsHome: g.IsHome}
	st.OpeningDays[dates.Season] = od
	s.saveSeasonState()
	return od
}

// countdownTask posts weekly on Sundays, then daily for the final week.
func (s *Scheduler) countdownTask(p PhaseInfo, r *tickResult) {
	if p.Upcoming == nil || p.Upcoming.RegularStart == "" {
		return
	}
	today := s.today()
	od := s.openingDayFor(p.Upcoming)
	days := daysBetween(today, od.Date)
	if days <= 0 {
		return
	}

	var key string
	var due time.Time
	if days <= 7 {
		key = "countdown:day:" + today
		due = clockOn(r.now, countdownHour, 0)
	} else {
		sunday := r.now.AddDate(0, 0, -int(r.now.Weekday()))
		key = "countdown:week:" + sunday.Format("2006-01-02")
		due = clockOn(sunday, countdownHour, 0)
	}

	if s.done(key) {
		next := clockOn(r.now.AddDate(0, 0, 1), countdownHour, 0)
		if days-1 > 7 {
			next = clockOn(r.now.AddDate(0, 0, 7-int(r.now.Weekday())), countdownHour, 0)
		}
		r.wakeAt(next)
		return
	}
	if r.now.Before(due) && !s.ignoreTimeGates {
		r.wakeAt(due)
		return
	}
	if !s.chatterAllowed(r) {
		return
	}

	horoText := ""
	if horo, err := s.horoscope.GetDailyHoroscope(); err != nil {
		s.logger.Printf("warning: could not get horoscope: %v", err)
	} else if horo != nil {
		horoText = horo.Text
	}

	post := formatter.FormatCountdown(formatter.Countdown{
		Days:          days,
		OpeningDay:    od.Date,
		Opponent:      od.Opponent,
		IsHome:        od.IsHome,
		SpringDays:    daysBetween(today, p.Upcoming.SpringStart),
		HoroscopeText: horoText,
	})
	if _, err := s.postOnce(key, post.Text, s.generateImage(post.HoroscopeText), "", ""); err != nil {
		r.fail("countdown", err)
	}
}
