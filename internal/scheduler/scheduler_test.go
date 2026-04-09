package scheduler

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
)

type mockMLB struct {
	game    *mlb.Game
	record  *mlb.TeamRecord
	pitcher *mlb.PitcherStats
	h2h     *mlb.H2HRecord
}

func (m *mockMLB) GetTodayGame() (*mlb.Game, error)                  { return m.game, nil }
func (m *mockMLB) GetTeamRecord() (*mlb.TeamRecord, error)           { return m.record, nil }
func (m *mockMLB) GetPitcherStats(id int) (*mlb.PitcherStats, error) { return m.pitcher, nil }
func (m *mockMLB) GetHeadToHead(id int) (*mlb.H2HRecord, error)     { return m.h2h, nil }

type mockHoroscope struct {
	horo *horoscope.Horoscope
}

func (m *mockHoroscope) GetDailyHoroscope() (*horoscope.Horoscope, error) {
	return m.horo, nil
}

type mockPoster struct {
	posts   []string
	replies []string
	seq     int
}

func (m *mockPoster) Post(text string) (*bluesky.PostRef, error) {
	m.seq++
	m.posts = append(m.posts, text)
	return &bluesky.PostRef{
		URI: fmt.Sprintf("at://mock/post/%d", m.seq),
		CID: fmt.Sprintf("mock-cid-%d", m.seq),
	}, nil
}

func (m *mockPoster) Reply(text string, parent bluesky.PostRef) (*bluesky.PostRef, error) {
	m.seq++
	m.replies = append(m.replies, text)
	return &bluesky.PostRef{
		URI: fmt.Sprintf("at://mock/post/%d", m.seq),
		CID: fmt.Sprintf("mock-cid-%d", m.seq),
	}, nil
}

func testLogger() *log.Logger {
	return log.New(os.Stderr, "[test] ", 0)
}

func TestTick_GameDay(t *testing.T) {
	denver := mlb.DenverLocation()
	gameTime := time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC)
	nowTime := gameTime.Add(-30 * time.Minute)

	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{
			game: &mlb.Game{
				GamePk:       1234,
				GameDateTime: gameTime,
				Status:       "Preview",
				Venue:        "Coors Field",
				IsHome:       true,
				HomeTeam: mlb.TeamInfo{
					ID: 115, Name: "Colorado Rockies", Wins: 5, Losses: 6,
					ProbablePitcher: &mlb.PitcherInfo{ID: 547179, FullName: "Michael Lorenzen"},
				},
				AwayTeam: mlb.TeamInfo{
					ID: 117, Name: "Houston Astros", Wins: 7, Losses: 4,
					ProbablePitcher: &mlb.PitcherInfo{ID: 621111, FullName: "Framber Valdez"},
				},
			},
			record:  &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455, StreakCode: "W2"},
			pitcher: &mlb.PitcherStats{FullName: "Michael Lorenzen", ERA: 9.00, Wins: 0, Losses: 1, GamesStarted: 3, InningsPitched: 13},
			h2h:     &mlb.H2HRecord{OpponentName: "Houston Astros", Wins: 2, Losses: 1, GamesPlayed: 3},
		},
		horoscope: &mockHoroscope{
			horo: &horoscope.Horoscope{Sign: "cancer", Text: "The stars favor bold moves today."},
		},
		poster: poster,
		now:    func() time.Time { return nowTime },
		sleep:  func(d time.Duration) {},
		logger: testLogger(),
	}

	err := s.Tick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(poster.posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.posts))
	}

	post := poster.posts[0]
	if !strings.Contains(post, "⚾ Rockies vs Houston Astros") {
		t.Errorf("missing matchup in post:\n%s", post)
	}
	if !strings.Contains(post, "🔮") {
		t.Errorf("missing prediction in post:\n%s", post)
	}

	// Should have a horoscope reply
	if len(poster.replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(poster.replies))
	}
	if !strings.Contains(poster.replies[0], "♋") {
		t.Errorf("missing cancer emoji in reply:\n%s", poster.replies[0])
	}
	if !strings.Contains(poster.replies[0], "bold moves") {
		t.Errorf("missing horoscope text in reply:\n%s", poster.replies[0])
	}

	today := nowTime.In(denver).Format("2006-01-02")
	if s.lastPostDate != today {
		t.Errorf("lastPostDate = %q, want %q", s.lastPostDate, today)
	}
}

func TestTick_OffDay(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 10, 11, 0, 0, 0, denver)

	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{
			game:   nil,
			record: &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455, RunDifferential: -3},
		},
		horoscope: &mockHoroscope{
			horo: &horoscope.Horoscope{Sign: "cancer", Text: "Rest and reflect on recent challenges."},
		},
		poster: poster,
		now:    func() time.Time { return nowTime },
		sleep:  func(d time.Duration) {},
		logger: testLogger(),
	}

	err := s.Tick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(poster.posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.posts))
	}

	if !strings.Contains(poster.posts[0], "⚾ No Rockies game today") {
		t.Errorf("missing off-day message:\n%s", poster.posts[0])
	}

	if len(poster.replies) != 1 {
		t.Fatalf("expected 1 horoscope reply, got %d", len(poster.replies))
	}
}

func TestTick_AlreadyPosted(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)

	poster := &mockPoster{}
	s := &Scheduler{
		mlb:          &mockMLB{game: nil},
		horoscope:    &mockHoroscope{},
		poster:       poster,
		now:          func() time.Time { return nowTime },
		sleep:        func(d time.Duration) {},
		logger:       testLogger(),
		lastPostDate: "2026-04-08",
	}

	err := s.Tick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(poster.posts) != 0 {
		t.Errorf("expected 0 posts (already posted), got %d", len(poster.posts))
	}
}

func TestTick_WaitsForGameTime(t *testing.T) {
	gameTime := time.Date(2026, 4, 8, 23, 0, 0, 0, time.UTC)
	nowTime := time.Date(2026, 4, 8, 15, 0, 0, 0, time.UTC)

	var firstSleep time.Duration
	sleepCount := 0
	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{
			game: &mlb.Game{
				GameDateTime: gameTime,
				Status:       "Preview",
				Venue:        "Coors Field",
				IsHome:       true,
				HomeTeam:     mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
				AwayTeam:     mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
			},
			record: &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455},
		},
		horoscope: &mockHoroscope{
			horo: &horoscope.Horoscope{Sign: "cancer", Text: "Patience is a virtue the cosmos rewards today."},
		},
		poster: poster,
		now:    func() time.Time { return nowTime },
		sleep: func(d time.Duration) {
			sleepCount++
			if sleepCount == 1 {
				firstSleep = d
			}
			nowTime = nowTime.Add(d)
		},
		logger: testLogger(),
	}

	err := s.Tick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSleep := gameTime.Add(-1 * time.Hour).Sub(time.Date(2026, 4, 8, 15, 0, 0, 0, time.UTC))
	if firstSleep != expectedSleep {
		t.Errorf("first sleep = %s, expected %s", firstSleep, expectedSleep)
	}

	if len(poster.posts) != 1 {
		t.Fatalf("expected 1 post after wait, got %d", len(poster.posts))
	}
}

func TestNextCheckTime(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)

	s := &Scheduler{
		now: func() time.Time { return nowTime },
	}

	next := s.nextCheckTime()
	expected := time.Date(2026, 4, 9, 5, 0, 0, 0, denver)
	if !next.Equal(expected) {
		t.Errorf("nextCheckTime = %v, want %v", next, expected)
	}
}

func TestTick_GameDay_NoHoroscope(t *testing.T) {
	gameTime := time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC)
	nowTime := gameTime.Add(-30 * time.Minute)

	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{
			game: &mlb.Game{
				GameDateTime: gameTime,
				Status:       "Preview",
				Venue:        "Coors Field",
				IsHome:       true,
				HomeTeam:     mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
				AwayTeam:     mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
			},
			record: &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455},
		},
		horoscope: &mockHoroscope{horo: nil},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
	}

	err := s.Tick()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(poster.posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.posts))
	}
	// No horoscope = no reply
	if len(poster.replies) != 0 {
		t.Errorf("expected 0 replies without horoscope, got %d", len(poster.replies))
	}
}
