package main

import (
	"fmt"
	"log"
	"os"
	"sort"

	"net/http"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
	"github.com/tlugger/rockiscope/internal/scheduler"
	"github.com/tlugger/rockiscope/internal/web"
)

var version = "dev"

const usage = `rockiscope — Colorado Rockies horoscope bot for Bluesky

Usage:
  rockiscope <command>

Commands:
  run          Start the scheduler daemon (default if no command given)
  serve        Start the analytics dashboard web server
  post         Force a post right now, skipping the schedule
  preview      Fetch all data and print the post without posting
  backfill     One-time: backfill missing season games and score data into prediction_history.json
  test-auth    Test Bluesky authentication
  test-mlb     Test MLB API connectivity and show today's game
  test-horo    Test horoscope scraper and show today's reading
  version      Print version

Environment:
  BLUESKY_USERNAME    Bluesky handle (e.g. yourname.bsky.social)
  BLUESKY_PASSWORD    Bluesky app password
  ROCKISCOPE_DATA_DIR Directory for persisted data (default: current dir)
`

func main() {
	logger := log.New(os.Stdout, "[rockiscope] ", log.LstdFlags)

	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		cmdRun(logger)
	case "serve":
		cmdServe(logger)
	case "post":
		cmdPost(logger)
	case "preview":
		cmdPreview(logger)
	case "backfill":
		cmdBackfill(logger)
	case "test-auth":
		cmdTestAuth(logger)
	case "test-mlb":
		cmdTestMLB(logger)
	case "test-horo":
		cmdTestHoro(logger)
	case "version":
		fmt.Printf("rockiscope %s\n", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		fmt.Print(usage)
		os.Exit(1)
	}
}

func cmdRun(logger *log.Logger) {
	logger.Printf("starting rockiscope %s", version)

	addr := ":8086"
	if port := os.Getenv("ROCKISCOPE_PORT"); port != "" {
		addr = ":" + port
	}
	dataDir := getDataDir()
	srv := web.NewServer(dataDir, logger)
	go func() {
		logger.Printf("analytics dashboard at http://localhost%s", addr)
		if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
			logger.Printf("warning: dashboard server error: %v", err)
		}
	}()

	poster := mustAuthBluesky(logger)
	sched := newScheduler(logger, poster)
	sched.Run()
}

func cmdServe(logger *log.Logger) {
	addr := ":8086"
	if port := os.Getenv("ROCKISCOPE_PORT"); port != "" {
		addr = ":" + port
	}
	dataDir := getDataDir()
	srv := web.NewServer(dataDir, logger)
	logger.Printf("analytics dashboard at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		logger.Fatalf("server error: %v", err)
	}
}

func cmdPost(logger *log.Logger) {
	poster := mustAuthBluesky(logger)
	sched := newScheduler(logger, poster)
	if err := sched.RunOnce(); err != nil {
		logger.Fatalf("post failed: %v", err)
	}
}

func cmdPreview(logger *log.Logger) {
	poster := &bluesky.DryRunPoster{
		OnPost: func(text string) {
			fmt.Println("─── Post ───")
			fmt.Println(text)
			fmt.Printf("─── %d chars ───\n", len(text))
		},
		OnImage: func(b []byte) {
			fmt.Printf("─── Image: %d bytes ───\n", len(b))
		},
	}
	sched := newScheduler(logger, poster)
	if err := sched.RunOnce(); err != nil {
		logger.Fatalf("preview failed: %v", err)
	}
}

func cmdBackfill(logger *log.Logger) {
	dataDir := getDataDir()
	hist, err := prediction.LoadHistory(dataDir)
	if err != nil {
		logger.Fatalf("loading history: %v", err)
	}

	client := mlb.NewClient(nil)
	logger.Println("fetching completed season games from MLB...")
	results, err := client.GetSeasonResults()
	if err != nil {
		logger.Fatalf("fetching season results: %v", err)
	}
	logger.Printf("found %d completed regular-season games", len(results))

	byGamePk := make(map[int]*prediction.PredictionRecord)
	byDateOpp := make(map[string]*prediction.PredictionRecord)
	for i := range hist.Predictions {
		p := &hist.Predictions[i]
		if p.GamePK != 0 {
			byGamePk[p.GamePK] = p
		}
		byDateOpp[p.Date+"|"+p.Opponent] = p
	}

	var created, scoresFilled, actualsFilled int
	for _, gr := range results {
		existing := byGamePk[gr.GamePk]
		if existing == nil {
			existing = byDateOpp[gr.Date+"|"+gr.Opponent]
		}
		actual := "L"
		if gr.Won {
			actual = "W"
		}

		if existing != nil {
			if existing.RockiesScore == 0 && existing.OppScore == 0 && (gr.RockiesScore != 0 || gr.OppScore != 0) {
				existing.RockiesScore = gr.RockiesScore
				existing.OppScore = gr.OppScore
				scoresFilled++
			}
			if existing.Actual == "" {
				existing.Actual = actual
				actualsFilled++
			}
			if existing.GamePK == 0 && gr.GamePk != 0 {
				existing.GamePK = gr.GamePk
			}
			continue
		}

		hist.Predictions = append(hist.Predictions, prediction.PredictionRecord{
			Date:           gr.Date,
			Opponent:       gr.Opponent,
			IsHome:         gr.IsHome,
			Predicted:      "L",
			Confidence:     50,
			Actual:         actual,
			RockiesScore:   gr.RockiesScore,
			OppScore:       gr.OppScore,
			GamePK:         gr.GamePk,
			WinProbability: 0.5,
			Synthetic:      true,
		})
		created++
	}

	sort.SliceStable(hist.Predictions, func(i, j int) bool {
		return hist.Predictions[i].Date < hist.Predictions[j].Date
	})

	if err := prediction.SaveHistory(hist, dataDir); err != nil {
		logger.Fatalf("saving history: %v", err)
	}
	logger.Printf("backfill complete: %d synthetic created, %d scores filled, %d actuals filled", created, scoresFilled, actualsFilled)
}

func cmdTestAuth(logger *log.Logger) {
	username, password := requireCreds(logger)
	logger.Printf("authenticating as %s...", username)

	client := bluesky.NewClient(username, password)
	if err := client.Authenticate(); err != nil {
		logger.Fatalf("authentication failed: %v", err)
	}
	logger.Println("authentication successful!")
}

func cmdTestMLB(logger *log.Logger) {
	client := mlb.NewClient(nil)

	logger.Println("fetching today's Rockies game...")
	game, err := client.GetTodayGame()
	if err != nil {
		logger.Fatalf("MLB API error: %v", err)
	}

	if game == nil {
		logger.Println("no game today (off day)")
	} else {
		fmt.Printf("Game:     %s %s %s\n", "Rockies", homeAway(game.IsHome), game.Opponent().Name)
		fmt.Printf("Time:     %s\n", game.FormatGameTime())
		fmt.Printf("Venue:    %s\n", game.Venue)
		fmt.Printf("Status:   %s\n", game.Status)
		if rp := game.RockiesPitcher(); rp != nil {
			fmt.Printf("Rockies SP: %s\n", rp.FullName)
		}
		if op := game.OpponentPitcher(); op != nil {
			fmt.Printf("Opp SP:     %s\n", op.FullName)
		}
	}

	logger.Println("fetching standings...")
	rec, err := client.GetTeamRecord()
	if err != nil {
		logger.Printf("standings error: %v", err)
	} else {
		fmt.Printf("Record:   %d-%d (%.3f)\n", rec.Wins, rec.Losses, rec.WinningPercentage)
		fmt.Printf("Streak:   %s\n", rec.StreakCode)
		fmt.Printf("Run Diff: %+d\n", rec.RunDifferential)
		fmt.Printf("Home:     %d-%d\n", rec.HomeWins, rec.HomeLosses)
		fmt.Printf("Away:     %d-%d\n", rec.AwayWins, rec.AwayLosses)
	}

	logger.Println("MLB API OK")
}

func cmdTestHoro(logger *log.Logger) {
	scraper := horoscope.NewScraper(nil)

	logger.Println("fetching Cancer horoscope...")
	horo, err := scraper.GetDailyHoroscope()
	if err != nil {
		logger.Fatalf("horoscope error: %v", err)
	}

	fmt.Printf("Sign: %s\n", horo.Sign)
	fmt.Printf("Text: %s\n", horo.Text)
	logger.Println("horoscope scraper OK")
}

func newScheduler(logger *log.Logger, poster bluesky.Poster) *scheduler.Scheduler {
	dataDir := getDataDir()
	return scheduler.New(scheduler.Config{
		MLB:       mlb.NewClient(nil),
		Horoscope: horoscope.NewScraper(nil),
		Poster:    poster,
		Logger:    logger,
		DataDir:   dataDir,
	})
}

func getDataDir() string {
	if dir := os.Getenv("ROCKISCOPE_DATA_DIR"); dir != "" {
		return dir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func mustAuthBluesky(logger *log.Logger) bluesky.Poster {
	username, password := requireCreds(logger)
	client := bluesky.NewClient(username, password)
	if err := client.Authenticate(); err != nil {
		logger.Fatalf("bluesky auth failed: %v", err)
	}
	logger.Printf("authenticated as %s", username)
	return client
}

func requireCreds(logger *log.Logger) (string, string) {
	username := os.Getenv("BLUESKY_USERNAME")
	password := os.Getenv("BLUESKY_PASSWORD")
	if username == "" || password == "" {
		logger.Fatal("BLUESKY_USERNAME and BLUESKY_PASSWORD must be set")
	}
	return username, password
}

func homeAway(isHome bool) string {
	if isHome {
		return "vs"
	}
	return "@"
}
