package main

import (
	"fmt"
	"log"
	"os"
	"time"
	// Embed the timezone database so Denver time works even on a minimal OS image.
	_ "time/tzdata"

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
  retry-post   Retry posting for today's predictions that failed to post to Bluesky
  preview      Fetch all data and print what would be posted, without posting or saving anything
  phase        Show the current season phase (regular season, postseason, offseason, spring)
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
	case "retry-post":
		cmdRetryPost(logger)
	case "preview":
		cmdPreview(logger)
	case "phase":
		cmdPhase(logger)
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

	username, password := requireCreds(logger)
	poster := bluesky.NewClient(username, password, logger)
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

func cmdRetryPost(logger *log.Logger) {
	poster := mustAuthBluesky(logger)
	sched := newScheduler(logger, poster)
	if err := sched.RetryPost(); err != nil {
		logger.Fatalf("retry-post failed: %v", err)
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
	sched := newSchedulerWith(logger, poster, true)
	if err := sched.RunOnce(); err != nil {
		logger.Fatalf("preview failed: %v", err)
	}
}

func cmdPhase(logger *log.Logger) {
	sched := newSchedulerWith(logger, &bluesky.DryRunPoster{}, true)
	p := sched.CurrentPhase()
	fmt.Printf("Phase:  %s\n", p.Phase)
	fmt.Printf("Season: %d\n", p.Season)
	fmt.Printf("Source: %s\n", p.Source)
	if p.Upcoming != nil {
		fmt.Printf("Next:   spring %s, opening day %s\n", p.Upcoming.SpringStart, p.Upcoming.RegularStart)
	}
}

func cmdBackfill(logger *log.Logger) {
	fromBluesky := false
	for _, arg := range os.Args[2:] {
		if arg == "--from-bluesky" {
			fromBluesky = true
		}
	}

	dataDir := getDataDir()
	hist, err := prediction.LoadHistory(dataDir)
	if err != nil {
		logger.Fatalf("loading history: %v", err)
	}

	client := mlb.NewClient(nil, logger)
	logger.Println("fetching completed season games from MLB...")
	results, err := client.GetSeasonResults()
	if err != nil {
		logger.Fatalf("fetching season results: %v", err)
	}
	logger.Printf("found %d completed regular-season games", len(results))

	// Leave the last couple of days alone so pending follow-up replies still fire.
	protectFrom := time.Now().In(mlb.DenverLocation()).AddDate(0, 0, -2).Format("2006-01-02")
	res := prediction.Reconcile(hist, results, protectFrom)

	if fromBluesky {
		username := os.Getenv("BLUESKY_USERNAME")
		if username == "" {
			logger.Fatal("BLUESKY_USERNAME must be set for --from-bluesky")
		}
		updated, err := bluesky.BackfillPredictionsFromBluesky(hist, username, results, logger)
		if err != nil {
			logger.Fatalf("bluesky backfill: %v", err)
		}
		logger.Printf("bluesky backfill: %d predictions updated", updated)
	}

	if err := prediction.SaveHistory(hist, dataDir); err != nil {
		logger.Fatalf("saving history: %v", err)
	}
	logger.Printf("backfill complete: %d synthetic created, %d scores filled, %d actuals filled", res.Created, res.ScoresFilled, res.ActualsFilled)
	logger.Println("the running bot merges these changes on its next wake; no restart needed")
}

func cmdTestAuth(logger *log.Logger) {
	username, password := requireCreds(logger)
	logger.Printf("authenticating as %s...", username)

	client := bluesky.NewClient(username, password, logger)
	if err := client.Authenticate(); err != nil {
		logger.Fatalf("authentication failed: %v", err)
	}
	logger.Println("authentication successful!")
}

func cmdTestMLB(logger *log.Logger) {
	client := mlb.NewClient(nil, logger)

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
	scraper := horoscope.NewScraper(nil, logger)

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
	return newSchedulerWith(logger, poster, false)
}

func newSchedulerWith(logger *log.Logger, poster bluesky.Poster, dryRun bool) *scheduler.Scheduler {
	client := mlb.NewClient(nil, logger)
	return scheduler.New(scheduler.Config{
		MLB:       client,
		Season:    client,
		Horoscope: horoscope.NewScraper(nil, logger),
		Poster:    poster,
		Logger:    logger,
		DataDir:   getDataDir(),
		DryRun:    dryRun,
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
	client := bluesky.NewClient(username, password, logger)
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
