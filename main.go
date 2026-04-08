package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/tlugger/rockiscope/internal/bluesky"
	"github.com/tlugger/rockiscope/internal/horoscope"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/scheduler"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "fetch data and print post without posting to Bluesky")
	flag.Parse()

	logger := log.New(os.Stdout, "[rockiscope] ", log.LstdFlags)

	mlbClient := mlb.NewClient(nil)
	horoscopeScraper := horoscope.NewScraper(nil)

	var poster bluesky.Poster
	if *dryRun {
		logger.Println("DRY RUN MODE — will not post to Bluesky")
		poster = &bluesky.DryRunPoster{
			OnPost: func(text string) {
				fmt.Println("--- POST ---")
				fmt.Println(text)
				fmt.Println("--- END ---")
			},
		}
	} else {
		username := os.Getenv("BLUESKY_USERNAME")
		password := os.Getenv("BLUESKY_PASSWORD")
		if username == "" || password == "" {
			logger.Fatal("BLUESKY_USERNAME and BLUESKY_PASSWORD must be set")
		}

		bsClient := bluesky.NewClient(username, password)
		if err := bsClient.Authenticate(); err != nil {
			logger.Fatalf("failed to authenticate with Bluesky: %v", err)
		}
		logger.Printf("authenticated as %s", username)
		poster = bsClient
	}

	sched := scheduler.New(scheduler.Config{
		MLB:       mlbClient,
		Horoscope: horoscopeScraper,
		Poster:    poster,
		Logger:    logger,
	})

	sched.Run()
}
