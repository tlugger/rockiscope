package scheduler

import (
	"fmt"
	"log"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/horoscope"
	imgcard "github.com/tlugger/rockiscope/internal/image"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

type Scheduler struct {
	mlb       mlb.GameProvider
	horoscope horoscope.Provider
	poster    bluesky.Poster
	now       func() time.Time
	sleep     func(time.Duration)
	logger    *log.Logger

	lastPostDate string
}

type Config struct {
	MLB       mlb.GameProvider
	Horoscope horoscope.Provider
	Poster    bluesky.Poster
	Logger    *log.Logger
}

func New(cfg Config) *Scheduler {
	return &Scheduler{
		mlb:       cfg.MLB,
		horoscope: cfg.Horoscope,
		poster:    cfg.Poster,
		now:       time.Now,
		sleep:     time.Sleep,
		logger:    cfg.Logger,
	}
}

func (s *Scheduler) Run() {
	s.logger.Println("rockiscope scheduler started")

	for {
		err := s.tick()
		if err != nil {
			s.logger.Printf("error: %v", err)
		}

		nextCheck := s.nextCheckTime()
		sleepDur := nextCheck.Sub(s.now())
		if sleepDur < 1*time.Minute {
			sleepDur = 1 * time.Minute
		}
		s.logger.Printf("sleeping until %s (%s)", nextCheck.Format("2006-01-02 15:04 MST"), sleepDur.Round(time.Minute))
		s.sleep(sleepDur)
	}
}

func (s *Scheduler) tick() error {
	denver := mlb.DenverLocation()
	today := s.now().In(denver).Format("2006-01-02")

	if today == s.lastPostDate {
		s.logger.Println("already posted today, skipping")
		return nil
	}

	game, err := s.mlb.GetTodayGame()
	if err != nil {
		return fmt.Errorf("fetching today's game: %w", err)
	}

	if game != nil {
		return s.handleGameDay(game, today)
	}
	return s.handleOffDay(today)
}

func (s *Scheduler) handleGameDay(game *mlb.Game, today string) error {
	denver := mlb.DenverLocation()

	postTime := game.GameDateTime.Add(-1 * time.Hour)
	now := s.now()

	if now.Before(postTime) {
		waitDur := postTime.Sub(now)
		s.logger.Printf("game at %s — posting at %s (in %s)",
			game.GameDateTime.In(denver).Format("3:04 PM MST"),
			postTime.In(denver).Format("3:04 PM MST"),
			waitDur.Round(time.Minute))
		s.sleep(waitDur)
	}

	post := s.buildGameDayPost(game)
	if err := s.publish(post); err != nil {
		return err
	}

	s.lastPostDate = today
	return nil
}

func (s *Scheduler) handleOffDay(today string) error {
	s.logger.Println("off day — posting horoscope + stats")

	denver := mlb.DenverLocation()
	offDayPostTime := time.Date(s.now().Year(), s.now().Month(), s.now().Day(), 10, 0, 0, 0, denver)
	now := s.now()

	if now.Before(offDayPostTime) {
		waitDur := offDayPostTime.Sub(now)
		s.logger.Printf("off day post at 10:00 AM MST (in %s)", waitDur.Round(time.Minute))
		s.sleep(waitDur)
	}

	post := s.buildOffDayPost()
	if err := s.publish(post); err != nil {
		return err
	}

	s.lastPostDate = today
	return nil
}

func (s *Scheduler) buildGameDayPost(game *mlb.Game) formatter.Post {
	s.logger.Println("gathering game data...")

	record, err := s.mlb.GetTeamRecord()
	if err != nil {
		s.logger.Printf("warning: could not get team record: %v", err)
	}

	var h2h *mlb.H2HRecord
	h2h, err = s.mlb.GetHeadToHead(game.OpponentID())
	if err != nil {
		s.logger.Printf("warning: could not get H2H: %v", err)
	}

	var pitcherStats *mlb.PitcherStats
	if rp := game.RockiesPitcher(); rp != nil {
		pitcherStats, err = s.mlb.GetPitcherStats(rp.ID)
		if err != nil {
			s.logger.Printf("warning: could not get pitcher stats: %v", err)
		}
	}

	horo, err := s.horoscope.GetDailyHoroscope()
	if err != nil {
		s.logger.Printf("warning: could not get horoscope: %v", err)
	}

	horoText := ""
	if horo != nil {
		horoText = horo.Text
	}

	pred := prediction.Predict(prediction.Input{
		Record:          record,
		RockiesPitcher:  pitcherStats,
		OpponentPitcher: s.getOpponentPitcher(game),
		HeadToHead:      h2h,
		IsHome:          game.IsHome,
		HoroscopeText:   horoText,
	})

	return formatter.FormatGameDay(formatter.GameDayPost{
		Game:       game,
		Record:     record,
		H2H:        h2h,
		Pitcher:    pitcherStats,
		Horoscope:  horo,
		Prediction: pred,
	})
}

func (s *Scheduler) buildOffDayPost() formatter.Post {
	record, err := s.mlb.GetTeamRecord()
	if err != nil {
		s.logger.Printf("warning: could not get team record: %v", err)
	}

	horo, err := s.horoscope.GetDailyHoroscope()
	if err != nil {
		s.logger.Printf("warning: could not get horoscope: %v", err)
	}

	return formatter.FormatOffDay(formatter.OffDayPost{
		Record:    record,
		Horoscope: horo,
	})
}

func (s *Scheduler) publish(post formatter.Post) error {
	s.logger.Printf("posting:\n%s", post.Text)

	var img *bluesky.ImageData
	if post.HoroscopeText != "" {
		pngBytes, w, h, err := imgcard.HoroscopeCard(post.HoroscopeText)
		if err != nil {
			s.logger.Printf("warning: could not generate horoscope image: %v", err)
		} else {
			s.logger.Printf("generated horoscope image (%d bytes, %dx%d)", len(pngBytes), w, h)
			img = &bluesky.ImageData{
				Bytes:  pngBytes,
				Alt:    "Today's Cancer horoscope: " + post.HoroscopeText,
				Width:  w,
				Height: h,
			}
		}
	}

	if _, err := s.poster.Post(post.Text, img); err != nil {
		return fmt.Errorf("posting to Bluesky: %w", err)
	}

	s.logger.Println("posted successfully!")
	return nil
}

func (s *Scheduler) getOpponentPitcher(game *mlb.Game) *mlb.PitcherStats {
	op := game.OpponentPitcher()
	if op == nil {
		return nil
	}
	stats, err := s.mlb.GetPitcherStats(op.ID)
	if err != nil {
		s.logger.Printf("warning: could not get opponent pitcher stats: %v", err)
		return nil
	}
	return stats
}

func (s *Scheduler) nextCheckTime() time.Time {
	denver := mlb.DenverLocation()
	now := s.now().In(denver)
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 5, 0, 0, 0, denver)
}

func (s *Scheduler) Tick() error {
	return s.tick()
}

func (s *Scheduler) RunOnce() error {
	game, err := s.mlb.GetTodayGame()
	if err != nil {
		return fmt.Errorf("fetching today's game: %w", err)
	}

	var post formatter.Post
	if game != nil {
		post = s.buildGameDayPost(game)
	} else {
		s.logger.Println("off day — posting horoscope + stats")
		post = s.buildOffDayPost()
	}

	return s.publish(post)
}
