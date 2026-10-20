package live

import (
	"sort"
	"strings"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/image"
	"github.com/tlugger/rockiscope/internal/mlb"
)

const (
	// pollEvery is the play-by-play cadence. Clips take 1-3 minutes to appear
	// anyway, so there's no point hammering the API at its suggested 10s.
	pollEvery = 30 * time.Second
	// contentEvery is how often to look for clips while plays are waiting.
	contentEvery = 45 * time.Second
	maxBackoff   = 5 * time.Minute
	// clipWait is how long a scoring play waits for its clip before posting
	// without one. Big swings that didn't score rarely get a quick clip, and
	// they hold up the queue, so they wait less.
	clipWait      = 10 * time.Minute
	clipWaitSwing = 4 * time.Minute
	// staleAfter skips plays that finished this long before we saw them (the
	// Pi was offline): no flood of old news after a reboot.
	staleAfter = 15 * time.Minute
	// clipMatchWindow bounds the fallback match for clips without a playId.
	clipMatchWindow = 15 * time.Minute
	// maxPostsPerGame is the live thread's own budget, separate from the
	// scheduler's daily cap so updates can never crowd out the result post.
	maxPostsPerGame = 15
	// maxPostFailures drops a play whose post keeps failing.
	maxPostFailures = 3
)

// watch follows one game until it's over and every queued play is posted.
// Network errors back off and retry; it only returns when the thread closes.
func (m *Manager) watch(gamePk int) {
	failures := 0
	var lastContent time.Time
	var highlights []mlb.Highlight
	for {
		t := m.snapshot(gamePk)
		if t == nil || t.Closed {
			return
		}
		if m.now().After(t.FirstPitch.Add(watchUntil)) {
			m.logger.Printf("game %d: watched long enough, closing the live thread", gamePk)
			m.update(gamePk, func(t *Thread) { t.Closed = true })
			return
		}

		game, err := m.mlb.GetLiveGame(gamePk)
		var plays []mlb.LivePlay
		if err == nil {
			plays, err = m.mlb.GetLivePlays(gamePk)
		}
		if err != nil {
			failures++
			wait := pollEvery << min(failures, 4)
			if wait > maxBackoff {
				wait = maxBackoff
			}
			m.logger.Printf("warning: live game %d: %v (retrying in %s)", gamePk, err, wait)
			m.sleep(wait)
			continue
		}
		failures = 0

		if gameCalledOff(game.DetailedState) {
			m.logger.Printf("game %d is %s, closing its live thread", gamePk, game.DetailedState)
			m.update(gamePk, func(t *Thread) { t.Closed = true })
			return
		}
		if game.Status == "Preview" {
			m.sleep(pollEvery)
			continue
		}

		m.ingest(gamePk, game, plays)

		if t = m.snapshot(gamePk); len(t.Pending) > 0 && m.now().Sub(lastContent) >= contentEvery {
			if hs, err := m.mlb.GetHighlights(gamePk); err != nil {
				m.logger.Printf("warning: highlights for game %d: %v", gamePk, err)
			} else {
				highlights, lastContent = hs, m.now()
			}
		}
		m.drain(gamePk, highlights)

		if t = m.snapshot(gamePk); game.Status == "Final" && len(t.Pending) == 0 {
			m.update(gamePk, func(t *Thread) { t.Closed = true })
			return
		}
		m.sleep(pollEvery)
	}
}

func gameCalledOff(detailed string) bool {
	for _, p := range []string{"Postponed", "Cancelled", "Suspended"} {
		if strings.HasPrefix(detailed, p) {
			return true
		}
	}
	return false
}

// ingest looks at newly completed plays: every one extends the chart, and
// notable fresh ones are queued for posting.
func (m *Manager) ingest(gamePk int, game *mlb.LiveGame, plays []mlb.LivePlay) {
	sort.Slice(plays, func(i, j int) bool { return plays[i].AtBatIndex < plays[j].AtBatIndex })
	// Most polls see nothing new; skip the write (the Pi's SD card thanks us).
	t := m.snapshot(gamePk)
	fresh := false
	for _, p := range plays {
		if p.IsComplete && p.AtBatIndex > t.LastAtBat {
			fresh = true
		}
	}
	if !fresh {
		return
	}
	now := m.now()
	m.update(gamePk, func(t *Thread) {
		isHome := game.Home.ID == t.TeamID
		t.AwayName, t.HomeName = formatter.Nickname(game.Away.Name), formatter.Nickname(game.Home.Name)
		for _, p := range plays {
			if !p.IsComplete || p.AtBatIndex <= t.LastAtBat {
				continue
			}
			t.LastAtBat = p.AtBatIndex

			if isNotable(p) {
				ended := p.EndTime
				if ended.IsZero() {
					ended = now
				}
				if now.Sub(ended) > staleAfter {
					m.logger.Printf("game %d: skipping stale play %d (%s), it ended %s ago", gamePk, p.AtBatIndex, p.Event, now.Sub(ended).Round(time.Minute))
				} else {
					r := evaluate(p, isHome, game.Players, t.Offset)
					t.Offset = r.Offset
					wait := clipWait
					if !p.IsScoringPlay && p.EventType != "home_run" {
						wait = clipWaitSwing
					}
					// Seeding by position in the thread rotates the reaction
					// pools, so back-to-back posts don't repeat a line.
					seq := t.Posts + len(t.Pending)
					t.Pending = append(t.Pending, Pending{
						AtBatIndex: p.AtBatIndex,
						PlayIDs:    p.PlayIDs,
						BatterID:   p.BatterID,
						EndTime:    ended,
						Deadline:   now.Add(wait),
						Update:     liveUpdate(t, game, p, r, seq),
						Fallback:   formatter.FirstSentence(p.Description),
					})
					m.logger.Printf("game %d: notable play %d queued (%s, WPA %+.1f, stars %.0f%%)", gamePk, p.AtBatIndex, p.Event, r.WPA, r.Cosmic)
				}
			}

			real := teamWP(p, isHome)
			t.Series = append(t.Series, image.WPPoint{
				Inning: p.Inning, IsTop: p.IsTop, Real: real, Cosmic: clampWP(real + t.Offset),
			})
		}
	})
}

func liveUpdate(t *Thread, game *mlb.LiveGame, p mlb.LivePlay, r Reading, seq int) formatter.LiveUpdate {
	return formatter.LiveUpdate{
		Team:          formatter.Nickname(t.TeamName),
		Inning:        p.Inning,
		IsTop:         p.IsTop,
		AwayAbbrev:    game.Away.Abbrev,
		HomeAbbrev:    game.Home.Abbrev,
		AwayScore:     p.AwayScore,
		HomeScore:     p.HomeScore,
		EventType:     p.EventType,
		IsScoringPlay: p.IsScoringPlay,
		Batter:        p.BatterName,
		Pitcher:       p.PitcherName,
		SignsKnown:    r.SignsKnown,
		Matchup:       r.Matchup,
		Destiny:       r.Destiny,
		HelpedUs:      r.WPA > 0,
		BatterWon:     r.BatterWon,
		WPA:           r.WPA,
		Leverage:      p.Leverage,
		LaunchSpeed:   p.LaunchSpeed,
		Distance:      p.Distance,
		Real:          r.Real,
		Cosmic:        r.Cosmic,
		PrevCosmic:    r.PrevCosmic,
		Seed:          t.GamePk + seq,
	}
}

// drain posts queued plays in order. The head of the queue posts once its
// clip shows up or its wait runs out; later plays wait their turn so the
// thread reads in game order.
func (m *Manager) drain(gamePk int, highlights []mlb.Highlight) {
	for {
		t := m.snapshot(gamePk)
		if t == nil || len(t.Pending) == 0 {
			return
		}
		if t.Posts >= maxPostsPerGame {
			m.logger.Printf("game %d: live post budget (%d) spent, dropping %d queued plays", gamePk, maxPostsPerGame, len(t.Pending))
			m.update(gamePk, func(t *Thread) { t.Pending = nil })
			return
		}
		head := t.Pending[0]
		h := matchClip(head, highlights)
		if h == nil && m.now().Before(head.Deadline) {
			return
		}

		u := head.Update
		var clip *bluesky.Clip
		if h != nil {
			u.Headline = h.Headline
			clip = &bluesky.Clip{
				VideoURL:    h.MP4URL,
				PageURL:     h.PageURL(),
				Title:       h.Headline,
				Description: h.Blurb,
				ThumbURL:    h.ThumbURL,
				Alt:         h.Blurb,
			}
			if clip.Alt == "" {
				clip.Alt = h.Headline
			}
		} else {
			u.Headline = head.Fallback
			m.logger.Printf("game %d: no clip for play %d, posting without one", gamePk, head.AtBatIndex)
		}

		ref, err := m.poster.ReplyClip(formatter.FormatLiveUpdate(u), clip, t.tail(), t.RootURI)
		if err != nil {
			m.logger.Printf("warning: live update for game %d play %d failed: %v", gamePk, head.AtBatIndex, err)
			m.update(gamePk, func(t *Thread) {
				if len(t.Pending) == 0 || t.Pending[0].AtBatIndex != head.AtBatIndex {
					return
				}
				if t.Pending[0].Failures++; t.Pending[0].Failures >= maxPostFailures {
					m.logger.Printf("game %d: giving up on play %d", gamePk, head.AtBatIndex)
					t.Pending = t.Pending[1:]
				}
			})
			return
		}
		m.update(gamePk, func(t *Thread) {
			t.TailURI = ref.URI
			t.Posts++
			if len(t.Pending) > 0 && t.Pending[0].AtBatIndex == head.AtBatIndex {
				t.Pending = t.Pending[1:]
			}
		})
	}
}

// matchClip finds a play's highlight: by playId when MLB tagged it (almost
// always), else by batter and timing for the occasional untagged clip.
func matchClip(p Pending, highlights []mlb.Highlight) *mlb.Highlight {
	ids := map[string]bool{}
	for _, id := range p.PlayIDs {
		ids[id] = true
	}
	for i := range highlights {
		if highlights[i].GUID != "" && ids[highlights[i].GUID] {
			return &highlights[i]
		}
	}
	for i := range highlights {
		h := &highlights[i]
		if h.GUID != "" || strings.HasPrefix(h.Headline, "Field View") || h.Date.Before(p.EndTime) || h.Date.After(p.EndTime.Add(clipMatchWindow)) {
			continue
		}
		for _, id := range h.PlayerIDs {
			if id == p.BatterID {
				return h
			}
		}
	}
	return nil
}
