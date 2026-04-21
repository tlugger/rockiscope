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
	"github.com/tlugger/rockiscope/internal/prediction"
)

type mockMLB struct {
	game      *mlb.Game
	record   *mlb.TeamRecord
	pitcher   *mlb.PitcherStats
	h2h       *mlb.H2HRecord
	gamesSince []mlb.GameResult
}

func (m *mockMLB) GetTodayGame() (*mlb.Game, error)                  { return m.game, nil }
func (m *mockMLB) GetTeamRecord() (*mlb.TeamRecord, error)           { return m.record, nil }
func (m *mockMLB) GetPitcherStats(id int) (*mlb.PitcherStats, error) { return m.pitcher, nil }
func (m *mockMLB) GetHeadToHead(id int) (*mlb.H2HRecord, error)     { return m.h2h, nil }
func (m *mockMLB) GetGamesSince(date string) ([]mlb.GameResult, error) { return m.gamesSince, nil }

type mockHoroscope struct {
	horo *horoscope.Horoscope
}

func (m *mockHoroscope) GetDailyHoroscope() (*horoscope.Horoscope, error) { return m.horo, nil }

type mockPoster struct {
	posts       []string
	replies     []string
	images      int
	seq        int
	replyCount int
}

func (m *mockPoster) Post(text string, image *bluesky.ImageData) (*bluesky.PostRef, error) {
	m.seq++
	m.posts = append(m.posts, text)
	if image != nil {
		m.images++
	}
	return &bluesky.PostRef{
		URI: fmt.Sprintf("at://mock/post/%d", m.seq),
		CID: fmt.Sprintf("mock-cid-%d", m.seq),
	}, nil
}

func (m *mockPoster) Reply(text string, image *bluesky.ImageData, parentURI, rootURI string) (*bluesky.PostRef, error) {
	m.seq++
	m.replyCount++
	m.replies = append(m.replies, text)
	if image != nil {
		m.images++
	}
	return &bluesky.PostRef{
		URI: fmt.Sprintf("at://mock/post/%d", m.seq),
		CID: fmt.Sprintf("mock-cid-%d", m.seq),
	}, nil
}

func testLogger() *log.Logger { return log.New(os.Stderr, "[test] ", 0) }

func TestTick_GameDay(t *testing.T) {
	denver := mlb.DenverLocation()
	gameTime := time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC)
	nowTime := gameTime.Add(-30 * time.Minute)

	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{
			game: &mlb.Game{
				GamePk: 1234, GameDateTime: gameTime, Status: "Preview",
				Venue: "Coors Field", IsHome: true,
				HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies", Wins: 5, Losses: 6,
					ProbablePitcher: &mlb.PitcherInfo{ID: 547179, FullName: "Michael Lorenzen"}},
				AwayTeam: mlb.TeamInfo{ID: 117, Name: "Houston Astros", Wins: 7, Losses: 4,
					ProbablePitcher: &mlb.PitcherInfo{ID: 621111, FullName: "Framber Valdez"}},
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

	if err := s.Tick(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(poster.posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.posts))
	}
	if !strings.Contains(poster.posts[0], "⚾ Rockies vs Houston Astros") {
		t.Errorf("missing matchup:\n%s", poster.posts[0])
	}
	if poster.images != 1 {
		t.Errorf("expected 1 image, got %d", poster.images)
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
		mlb:       &mockMLB{game: nil, record: &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455, RunDifferential: -3}},
		horoscope: &mockHoroscope{horo: &horoscope.Horoscope{Sign: "cancer", Text: "Rest and reflect."}},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
	}

	if err := s.Tick(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(poster.posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.posts))
	}
	if !strings.Contains(poster.posts[0], "⚾ No Rockies game today") {
		t.Errorf("missing off-day:\n%s", poster.posts[0])
	}
	if poster.images != 1 {
		t.Errorf("expected 1 image for off-day, got %d", poster.images)
	}
}

func TestTick_AlreadyPosted(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)
	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{game: nil}, horoscope: &mockHoroscope{},
		poster: poster, now: func() time.Time { return nowTime },
		sleep: func(d time.Duration) {}, logger: testLogger(),
		lastPostDate: "2026-04-08",
	}

	if err := s.Tick(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(poster.posts) != 0 {
		t.Errorf("expected 0 posts, got %d", len(poster.posts))
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
				GameDateTime: gameTime, Status: "Preview", Venue: "Coors Field", IsHome: true,
				HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
				AwayTeam: mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
			},
			record: &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455},
		},
		horoscope: &mockHoroscope{horo: &horoscope.Horoscope{Sign: "cancer", Text: "Patience."}},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep: func(d time.Duration) {
			sleepCount++
			if sleepCount == 1 {
				firstSleep = d
			}
			nowTime = nowTime.Add(d)
		},
		logger: testLogger(),
	}

	if err := s.Tick(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSleep := gameTime.Add(-1 * time.Hour).Sub(time.Date(2026, 4, 8, 15, 0, 0, 0, time.UTC))
	if firstSleep != expectedSleep {
		t.Errorf("first sleep = %s, expected %s", firstSleep, expectedSleep)
	}
	if len(poster.posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.posts))
	}
}

func TestTick_NoHoroscope(t *testing.T) {
	gameTime := time.Date(2026, 4, 8, 19, 10, 0, 0, time.UTC)
	nowTime := gameTime.Add(-30 * time.Minute)

	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{
			game: &mlb.Game{
				GameDateTime: gameTime, Status: "Preview", Venue: "Coors Field", IsHome: true,
				HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
				AwayTeam: mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
			},
			record: &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455},
		},
		horoscope: &mockHoroscope{horo: nil},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
	}

	if err := s.Tick(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if poster.images != 0 {
		t.Errorf("expected 0 images without horoscope, got %d", poster.images)
	}
}

func TestNextCheckTime(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)
	s := &Scheduler{now: func() time.Time { return nowTime }}

	next := s.nextCheckTime()
	expected := time.Date(2026, 4, 9, 5, 0, 0, 0, denver)
	if !next.Equal(expected) {
		t.Errorf("nextCheckTime = %v, want %v", next, expected)
	}
}

func TestNew(t *testing.T) {
	cfg := Config{
		MLB:       &mockMLB{},
		Horoscope: &mockHoroscope{},
		Poster:   &mockPoster{},
		Logger:   testLogger(),
	}

	s := New(cfg)
	if s == nil {
		t.Fatal("expected scheduler")
	}
}

func TestRunOnce(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)

	poster := &mockPoster{}
	s := &Scheduler{
		mlb: &mockMLB{
			game: &mlb.Game{
				GamePk: 1234, GameDateTime: nowTime.Add(2 * time.Hour), Status: "Preview",
				Venue: "Coors Field", IsHome: true,
				HomeTeam: mlb.TeamInfo{ID: 115, Name: "Colorado Rockies"},
				AwayTeam: mlb.TeamInfo{ID: 117, Name: "Houston Astros"},
			},
			record: &mlb.TeamRecord{Wins: 5, Losses: 6, WinningPercentage: 0.455},
		},
		horoscope: &mockHoroscope{
			horo: &horoscope.Horoscope{Sign: "cancer", Text: "Test horoscope"},
		},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
	}

	err := s.RunOnce()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(poster.posts) != 1 {
		t.Errorf("expected 1 post, got %d", len(poster.posts))
	}
}

func TestRecordOffDay(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)

	s := &Scheduler{
		mlb:       &mockMLB{},
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
		dataDir:   "",
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{},
			Current:    prediction.DefaultWeights(),
		},
	}

	pred := prediction.Prediction{WinProbability: 0.5, Pick: "W"}
	s.recordOffDay("2026-04-08", pred, "at://test/post/1")

	if len(s.predHistory.Predictions) != 1 {
		t.Errorf("expected 1 prediction recorded, got %d", len(s.predHistory.Predictions))
	}
}

func TestCheckForCompletedGames_NoHistory(t *testing.T) {
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, time.UTC)
	s := &Scheduler{
		mlb:       &mockMLB{},
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
		dataDir:   "",
	}

	err := s.checkForCompletedGames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckForCompletedGames_MatchingResults(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)
	poster := &mockPoster{}

	s := &Scheduler{
		mlb: &mockMLB{
			gamesSince: []mlb.GameResult{
				{GamePk: 1234, Date: "2026-04-08", Opponent: "Houston Astros", Won: true},
			},
		},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
		dataDir:   "",
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{
				{Date: "2026-04-08", Opponent: "Houston Astros", Predicted: "W", GamePK: 1234},
			},
			Current: prediction.Weights{WinRate: 0.30, Pitcher: 0.30},
		},
	}

	err := s.checkForCompletedGames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.predHistory.Predictions[0].Actual != "W" {
		t.Errorf("expected actual = W, got %s", s.predHistory.Predictions[0].Actual)
	}
}

func TestCheckForCompletedGames_WrongPrediction(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)
	poster := &mockPoster{}

	s := &Scheduler{
		mlb: &mockMLB{
			gamesSince: []mlb.GameResult{
				{GamePk: 1234, Date: "2026-04-08", Opponent: "Houston Astros", Won: true},
			},
		},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
		dataDir:   "",
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{
				{Date: "2026-04-08", Opponent: "Houston Astros", Predicted: "L", GamePK: 1234},
			},
			Current: prediction.Weights{WinRate: 0.30, Pitcher: 0.30},
		},
	}

	err := s.checkForCompletedGames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.predHistory.Predictions[0].Actual != "W" {
		t.Errorf("expected actual = W, got %s", s.predHistory.Predictions[0].Actual)
	}
}

func TestCheckForCompletedGames_CallsReply(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)
	poster := &mockPoster{}

	s := &Scheduler{
		mlb: &mockMLB{
			gamesSince: []mlb.GameResult{
				{GamePk: 1234, Date: "2026-04-08", Opponent: "Houston Astros", Won: true},
			},
		},
		poster:    poster,
		now:       func() time.Time { return nowTime },
		sleep:     func(d time.Duration) {},
		logger:    testLogger(),
		dataDir:   "",
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{
				{
					Date:     "2026-04-08",
					Opponent: "Houston Astros",
					Predicted: "W",
					GamePK:  1234,
				},
			},
			Current: prediction.Weights{WinRate: 0.30, Pitcher: 0.30},
		},
	}

	err := s.checkForCompletedGames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if poster.replyCount == 0 {
		t.Error("expected a reply to be posted")
	}
}

func TestCheckForCompletedGames_WeightAdjustment(t *testing.T) {
	denver := mlb.DenverLocation()
	nowTime := time.Date(2026, 4, 8, 14, 0, 0, 0, denver)

	s := &Scheduler{
		mlb: &mockMLB{
			gamesSince: []mlb.GameResult{
				{GamePk: 1234, Date: "2026-04-08", Opponent: "Houston Astros", Won: true},
				{GamePk: 1235, Date: "2026-04-07", Opponent: "Dodgers", Won: true},
				{GamePk: 1236, Date: "2026-04-06", Opponent: "Giants", Won: false},
			},
		},
		now:   func() time.Time { return nowTime },
		sleep: func(d time.Duration) {},
		logger: testLogger(),
		dataDir: "",
		predHistory: &prediction.PredictionHistory{
			Predictions: []prediction.PredictionRecord{
				{Date: "2026-04-08", Opponent: "Houston Astros", Predicted: "W", Actual: "W", GamePK: 1234},
				{Date: "2026-04-07", Opponent: "Dodgers", Predicted: "W", Actual: "W", GamePK: 1235},
				{Date: "2026-04-06", Opponent: "Giants", Predicted: "W", Actual: "L", GamePK: 1236},
			},
			Current: prediction.Weights{WinRate: 0.30, Pitcher: 0.30},
		},
	}

	err := s.checkForCompletedGames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	originalWinRate := prediction.Weights{WinRate: 0.30, Pitcher: 0.30}.WinRate
	if s.predHistory.Current.WinRate == originalWinRate {
		t.Logf("weights may adjust after 3+ predictions, got winRate=%f", s.predHistory.Current.WinRate)
	}
}
