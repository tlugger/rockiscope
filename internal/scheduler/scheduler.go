package scheduler

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/horoscope"
	imgcard "github.com/tlugger/rockiscope/internal/image"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

type Scheduler struct {
	mlb        mlb.GameProvider
	horoscope  horoscope.Provider
	poster     bluesky.Poster
	now        func() time.Time
	sleep      func(time.Duration)
	logger     *log.Logger
	dataDir    string

	lastPostDate   string
	lastReplyDate string
	predHistory   *prediction.PredictionHistory
}

type Config struct {
	MLB       mlb.GameProvider
	Horoscope horoscope.Provider
	Poster    bluesky.Poster
	Logger    *log.Logger
	DataDir   string
}

func New(cfg Config) *Scheduler {
	s := &Scheduler{
		mlb:       cfg.MLB,
		horoscope: cfg.Horoscope,
		poster:    cfg.Poster,
		now:       time.Now,
		sleep:     time.Sleep,
		logger:    cfg.Logger,
		dataDir:   cfg.DataDir,
	}
	loadLastPostDate(s)
	loadLastReplyDate(s)
	loadPredictionHistory(s)
	return s
}

func (s *Scheduler) lastPostDateFile() string {
	return filepath.Join(s.dataDir, "last_post_date")
}

func loadLastPostDate(s *Scheduler) {
	denver := mlb.DenverLocation()
	today := s.now().In(denver).Format("2006-01-02")

	if s.dataDir == "" {
		s.logger.Println("no data dir configured, defaulting to today's date")
		s.lastPostDate = today
		return
	}

	data, err := os.ReadFile(s.lastPostDateFile())
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Println("no last post date found, defaulting to today's date")
		} else {
			s.logger.Printf("warning: could not read last post date: %v", err)
		}
		s.lastPostDate = today
		return
	}
	s.lastPostDate = string(data)
	s.logger.Printf("loaded last post date: %s", s.lastPostDate)
}

func saveLastPostDate(s *Scheduler) error {
	if s.dataDir == "" {
		s.logger.Println("no data dir configured, not saving")
		return nil
	}

	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	if err := os.WriteFile(s.lastPostDateFile(), []byte(s.lastPostDate), 0644); err != nil {
		return fmt.Errorf("saving last post date: %w", err)
	}
	s.logger.Printf("saved last post date: %s", s.lastPostDate)
	return nil
}

func (s *Scheduler) lastReplyDateFile() string {
	return filepath.Join(s.dataDir, "last_reply_date")
}

func loadLastReplyDate(s *Scheduler) {
	denver := mlb.DenverLocation()
	today := s.now().In(denver).Format("2006-01-02")

	if s.dataDir == "" {
		s.lastReplyDate = today
		return
	}

	data, err := os.ReadFile(s.lastReplyDateFile())
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Println("no last reply date found")
		} else {
			s.logger.Printf("warning: could not read last reply date: %v", err)
		}
		s.lastReplyDate = ""
		return
	}
	s.lastReplyDate = string(data)
	s.logger.Printf("loaded last reply date: %s", s.lastReplyDate)
}

func saveLastReplyDate(s *Scheduler) error {
	if s.dataDir == "" {
		s.logger.Println("no data dir configured, not saving")
		return nil
	}

	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	if err := os.WriteFile(s.lastReplyDateFile(), []byte(s.lastReplyDate), 0644); err != nil {
		return fmt.Errorf("saving last reply date: %w", err)
	}
	s.logger.Printf("saved last reply date: %s", s.lastReplyDate)
	return nil
}

func loadPredictionHistory(s *Scheduler) {
	hist, err := prediction.LoadHistory(s.dataDir)
	if err != nil {
		s.logger.Printf("warning: could not load prediction history: %v", err)
		s.predHistory = &prediction.PredictionHistory{
			Predictions: nil,
			Current:    prediction.DefaultWeights(),
		}
		return
	}

	if len(hist.Predictions) == 0 {
		s.logger.Println("first run - starting fresh with default weights")
		s.logger.Println("prediction history will accumulate over the season")
	}

	s.predHistory = hist
	s.logger.Printf("prediction engine: %d games recorded, %d correct",
		len(hist.Predictions), hist.CorrectCount())
}

func (s *Scheduler) savePredictionHistory() error {
	if s.dataDir == "" {
		s.logger.Println("no data dir configured, not saving prediction history")
		return nil
	}
	if s.predHistory == nil {
		return nil
	}
	return prediction.SaveHistory(s.predHistory, s.dataDir)
}

func (s *Scheduler) Run() {
	s.logger.Println("rockiscope scheduler started")

	for {
		s.logger.Println("checking for completed games...")
		if err := s.checkForCompletedGames(); err != nil {
			s.logger.Printf("warning: could not check completed games: %v", err)
		}

		err := s.tick()
		if err != nil {
			s.logger.Printf("error: %v", err)
		}

		sleepDur := s.calculateSleepDuration()
		s.logger.Printf("sleeping for %s", sleepDur.Round(time.Minute))
		s.sleep(sleepDur)
	}
}

func (s *Scheduler) calculateSleepDuration() time.Duration {
	denver := mlb.DenverLocation()
	now := s.now().In(denver)
	today := now.Format("2006-01-02")

	games, err := s.mlb.GetTodayGames()
	if err == nil && len(games) > 0 {
		if s.lastPostDate == today && s.lastReplyDate != today {
			for _, game := range games {
				if !s.hasPostedGame(game.GamePk) && game.Status != "Final" {
					gameTime := now
					if game.GameDateTime.After(now) {
						gameTime = game.GameDateTime.In(denver)
					}
					wakeTime := gameTime.Add(30 * time.Minute)
					if wakeTime.After(now) {
						return wakeTime.Sub(now)
					}
					return 30 * time.Minute
				}
			}
			return 1 * time.Minute
		}

		if s.lastPostDate != today {
			earliestGameTime := time.Time{}
			for _, game := range games {
				if s.hasPostedGame(game.GamePk) {
					continue
				}
				gameTime := game.GameDateTime.In(denver)
				if earliestGameTime.IsZero() || gameTime.Before(earliestGameTime) {
					earliestGameTime = gameTime
				}
			}
			if !earliestGameTime.IsZero() && earliestGameTime.After(now) {
				wakeTime := earliestGameTime.Add(-1 * time.Hour)
				if wakeTime.After(now) {
					return wakeTime.Sub(now)
				}
			}
		}
	}

	nextCheck := s.nextCheckTime()
	sleepDur := nextCheck.Sub(now)
	if sleepDur < 1*time.Minute {
		sleepDur = 1 * time.Minute
	}
	return sleepDur
}

func (s *Scheduler) tick() error {
	denver := mlb.DenverLocation()
	today := s.now().In(denver).Format("2006-01-02")

	games, err := s.mlb.GetTodayGames()
	if err != nil {
		return fmt.Errorf("fetching today's games: %w", err)
	}

	if len(games) > 0 {
		anyGamePosted := false
		for _, game := range games {
			isFinal := game.Status == "Final"

			if isFinal && s.lastReplyDate != today {
				s.logger.Println("game is final, posting reply...")
				if err := s.handleGameReply(game, today); err != nil {
					return err
				}
			}

			alreadyPosted := s.hasPostedGame(game.GamePk)
			if !alreadyPosted && game.IsPlayable() {
				if err := s.handleGameDay(game, today); err != nil {
					return err
				}
				anyGamePosted = true
				continue
			}

			s.logger.Printf("game %d already posted, skipping", game.GamePk)
		}

		if anyGamePosted {
			s.logger.Printf("posted predictions for today's games")
		} else if s.lastPostDate == today {
			s.logger.Printf("already posted today (waiting for final)")
		}
		return nil
	}

	if s.lastPostDate != today {
		return s.handleOffDay(today)
	}

	s.logger.Println("already posted today, skipping")
	return nil
}

func (s *Scheduler) handleGameReply(game *mlb.Game, today string) error {
	denver := mlb.DenverLocation()
	todayDate := s.now().In(denver).Format("2006-01-02")

	s.logger.Println("checking completed games...")

	games, err := s.mlb.GetGamesSince(s.lastPostDate)
	if err != nil {
		return fmt.Errorf("fetching completed games: %w", err)
	}

	for _, gr := range games {
		s.processCompletedGame(gr, todayDate)
	}

	hasPendingReply := false
	for _, p := range s.predHistory.Predictions {
		if p.Actual != "" && p.PostURI != "" && p.Date == todayDate {
			hasPendingReply = true
			break
		}
	}

	if hasPendingReply {
		s.lastReplyDate = todayDate
		if err := saveLastReplyDate(s); err != nil {
			s.logger.Printf("warning: could not save reply date: %v", err)
		}
	}

	return nil
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
	img := s.generateImage(post.HoroscopeText)
	postRef, err := s.poster.Post(post.Text, img)
	if err != nil {
		return err
	}

	s.recordPrediction(today, game, post.Prediction, postRef.URI)

	s.lastPostDate = today
	if err := saveLastPostDate(s); err != nil {
		s.logger.Printf("warning: could not save last post date: %v", err)
	}
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
	img := s.generateImage(post.HoroscopeText)
	postRef, err := s.poster.Post(post.Text, img)
	if err != nil {
		return err
	}

	s.recordOffDay(today, post.Prediction, postRef.URI)

	s.lastPostDate = today
	if err := saveLastPostDate(s); err != nil {
		s.logger.Printf("warning: could not save last post date: %v", err)
	}
	return nil
}

func (s *Scheduler) generateImage(horoText string) *bluesky.ImageData {
	if horoText == "" {
		return nil
	}
	pngBytes, w, h, err := imgcard.HoroscopeCard(horoText)
	if err != nil {
		s.logger.Printf("warning: could not generate horoscope image: %v", err)
		return nil
	}
	s.logger.Printf("generated horoscope image (%d bytes, %dx%d)", len(pngBytes), w, h)
	return &bluesky.ImageData{
		Bytes:  pngBytes,
		Alt:    "Today's Cancer horoscope: " + horoText,
		Width:  w,
		Height: h,
	}
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

	weights := prediction.DefaultWeights()
	if s.predHistory != nil {
		weights = s.predHistory.Current
	}

	pred := prediction.Predict(prediction.Input{
		Record:          record,
		RockiesPitcher:  pitcherStats,
		OpponentPitcher: s.getOpponentPitcher(game),
		HeadToHead:      h2h,
		IsHome:          game.IsHome,
		HoroscopeText:   horoText,
	}, weights)

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

func (s *Scheduler) recordPrediction(date string, game *mlb.Game, pred prediction.Prediction, postURI string) {
	factors := prediction.FactorScores{}
	if pred.Factors != nil {
		if v, ok := pred.Factors["winRate"]; ok {
			factors.WinRate = v
		}
		if v, ok := pred.Factors["pitcher"]; ok {
			factors.Pitcher = v
		}
		if v, ok := pred.Factors["h2h"]; ok {
			factors.H2H = v
		}
		if v, ok := pred.Factors["homeAway"]; ok {
			factors.HomeAway = v
		}
		if v, ok := pred.Factors["momentum"]; ok {
			factors.Momentum = v
		}
		if v, ok := pred.Factors["stars"]; ok {
			factors.Stars = v
		}
	}
	rec := prediction.PredictionRecord{
		Date:            date,
		Opponent:        game.Opponent().Name,
		IsHome:          game.IsHome,
		Predicted:       pred.Pick,
		Confidence:     pred.WinProbability * 100,
		PostURI:         postURI,
		GamePK:          game.GamePk,
		WinProbability: pred.WinProbability,
		Factors:       factors,
	}

	if s.predHistory == nil {
		return
	}
	s.predHistory.Add(rec)
	if err := s.savePredictionHistory(); err != nil {
		s.logger.Printf("warning: could not save prediction: %v", err)
	}
}

func (s *Scheduler) recordOffDay(date string, pred prediction.Prediction, postURI string) {
	rec := prediction.PredictionRecord{
		Date:            date,
		Opponent:        "Off Day",
		IsHome:          false,
		Predicted:       "N/A",
		Confidence:     0,
		PostURI:         postURI,
		GamePK:         0,
		WinProbability: pred.WinProbability,
	}

	if s.predHistory == nil {
		return
	}
	s.predHistory.Add(rec)
	if err := s.savePredictionHistory(); err != nil {
		s.logger.Printf("warning: could not save prediction: %v", err)
	}
}

func (s *Scheduler) Tick() error {
	s.logger.Println("checking for completed games to update predictions...")
	if err := s.checkForCompletedGames(); err != nil {
		s.logger.Printf("warning: could not check completed games: %v", err)
	}
	return s.tick()
}

func (s *Scheduler) checkForCompletedGames() error {
	if s.predHistory == nil || len(s.predHistory.Predictions) == 0 {
		s.logger.Println("no predictions to check")
		return nil
	}

	denver := mlb.DenverLocation()
	today := s.now().In(denver).Format("2006-01-02")

	games, err := s.mlb.GetGamesSince(s.lastPostDate)
	if err != nil {
		return fmt.Errorf("fetching completed games: %w", err)
	}

	newResults := false
	for _, gr := range games {
		if s.processCompletedGame(gr, today) {
			newResults = true
		}
	}

	if newResults {
		completed := s.predHistory.Completed()
		totalGames := len(completed)
		accuracy, sampleCounts := prediction.CalculateFactorAccuracy(completed)
		newWeights := prediction.AdjustWeights(s.predHistory.Current, accuracy, sampleCounts, totalGames)
		s.predHistory.Current = newWeights
		s.logger.Printf("adjusted weights (%d games): winRate=%.0f%% pitcher=%.0f%% h2h=%.0f%% homeAway=%.0f%% momentum=%.0f%% stars=%.0f%%",
			totalGames,
			newWeights.WinRate*100, newWeights.Pitcher*100, newWeights.H2H*100,
			newWeights.HomeAway*100, newWeights.Momentum*100, newWeights.Stars*100)
		if err := s.savePredictionHistory(); err != nil {
			s.logger.Printf("warning: could not save weights: %v", err)
		}
	}

	return nil
}

func (s *Scheduler) processCompletedGame(gr mlb.GameResult, today string) bool {
	if s.predHistory == nil {
		return false
	}

	found := false
	for i := range s.predHistory.Predictions {
		p := &s.predHistory.Predictions[i]
		if p.Actual != "" {
			continue
		}
		if p.GamePK == gr.GamePk || (p.Date == gr.Date && p.Opponent == gr.Opponent) {
			actual := "L"
			if gr.Won {
				actual = "W"
			}

			correct := p.Predicted == actual
			resultStatus := "wrong"
			if correct {
				resultStatus = "correct"
			}
			s.logger.Printf("game result: %s %s %d-%d -> predicted %s, %s",
				p.Opponent, actual, gr.RockiesScore, gr.OppScore, p.Predicted, resultStatus)

			record := fmt.Sprintf("%d/%d correct predictions this season",
				s.predHistory.CorrectCount(), s.predHistory.TotalCount())

			score := fmt.Sprintf("Colorado Rockies %d-%d %s",
				gr.RockiesScore, gr.OppScore, p.Opponent)

			if err := s.postFollowUp(p.PostURI, correct, actual, score, record); err != nil {
				s.logger.Printf("warning: could not post follow-up: %v", err)
			} else {
				p.Actual = actual
				p.RockiesScore = gr.RockiesScore
				p.OppScore = gr.OppScore
				found = true
			}
			break
		}
	}

	if !found {
		s.logger.Printf("no matching prediction for game: %s %d-%d", gr.Opponent, gr.RockiesScore, gr.OppScore)
	}
	return found
}

func (s *Scheduler) postFollowUp(parentURI string, correct bool, actual, score, record string) error {
	outcome := fmt.Sprintf("Rockies %s", actual)

	text := formatter.FormatFollowUp(formatter.FollowUp{
		Outcome: outcome,
		Score:   score,
		Correct: correct,
		Record:  record,
	})

	_, err := s.poster.Reply(text, nil, parentURI, parentURI)
	if err != nil {
		return fmt.Errorf("posting follow-up: %w", err)
	}

	s.logger.Println("posted follow-up!")
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

func (s *Scheduler) hasPostedGame(gamePk int) bool {
	if s.predHistory == nil {
		return false
	}
	for _, p := range s.predHistory.Predictions {
		if p.GamePK == gamePk {
			return true
		}
	}
	return false
}

func (s *Scheduler) nextCheckTime() time.Time {
	denver := mlb.DenverLocation()
	now := s.now().In(denver)
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 5, 0, 0, 0, denver)
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
