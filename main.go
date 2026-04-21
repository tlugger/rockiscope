package main

import (
	"fmt"
	"log"
	"os"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/scheduler"
)

var version = "dev"

const usage = `rockiscope — Colorado Rockies horoscope bot for Bluesky

Usage:
  rockiscope <command>

Commands:
  run          Start the scheduler daemon (default if no command given)
  post         Force a post right now, skipping the schedule
  preview      Fetch all data and print the post without posting
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
	case "post":
		cmdPost(logger)
	case "preview":
		cmdPreview(logger)
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
	poster := mustAuthBluesky(logger)
	sched := newScheduler(logger, poster)
	sched.Run()
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
