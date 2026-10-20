package mlb

import (
	"testing"
	"time"
)

// Fixtures are ALCS Game 5, 2026 (Rays 3, Guardians 1), recorded from the
// live API with the same fields filters the client sends.
func liveFixtureClient(t *testing.T) *Client {
	return fixtureClient(t, map[string]string{
		"/game/849817/feed/live":      "live_849817_feed.json",
		"/game/849817/winProbability": "live_849817_winprob.json",
		"/game/849817/content":        "live_849817_content.json",
	})
}

func TestGetLiveGame(t *testing.T) {
	g, err := liveFixtureClient(t).GetLiveGame(849817)
	if err != nil {
		t.Fatal(err)
	}
	if g.Status != "Final" || g.Away.Abbrev != "TB" || g.Home.ID != 114 {
		t.Errorf("game = %+v", g)
	}
	if p := g.Players[666018]; p.FullName != "Jonathan Aranda" || p.BirthDate != "1998-05-23" {
		t.Errorf("Aranda = %+v", p)
	}
}

func TestGetLivePlays(t *testing.T) {
	plays, err := liveFixtureClient(t).GetLivePlays(849817)
	if err != nil {
		t.Fatal(err)
	}
	if len(plays) != 72 {
		t.Fatalf("got %d plays, want 72", len(plays))
	}
	p := plays[14]
	if p.AtBatIndex != 14 || !p.IsScoringPlay || !p.IsTop || p.Inning != 2 || p.EventType != "single" {
		t.Errorf("play 14 = %+v", p)
	}
	if p.BatterID != 666018 || p.PitcherName != "Parker Messick" {
		t.Errorf("matchup = %d vs %s", p.BatterID, p.PitcherName)
	}
	if last := p.PlayIDs[len(p.PlayIDs)-1]; last != "60e4a156-afe8-306d-875f-07eef45d7f8a" {
		t.Errorf("last playId = %s", last)
	}
	if p.LaunchSpeed != 85.5 || p.HomeWPA > -8 || p.Leverage != 1.79 || p.AwayScore != 2 {
		t.Errorf("stats = %+v", p)
	}
	if want := time.Date(2026, 10, 18, 21, 37, 3, 224e6, time.UTC); !p.EndTime.Equal(want) {
		t.Errorf("end = %s", p.EndTime)
	}
}

func TestGetHighlights(t *testing.T) {
	hs, err := liveFixtureClient(t).GetHighlights(849817)
	if err != nil {
		t.Fatal(err)
	}
	var aranda *Highlight
	for i := range hs {
		if hs[i].GUID == "60e4a156-afe8-306d-875f-07eef45d7f8a" {
			aranda = &hs[i]
		}
		if hs[i].MP4URL == "" {
			t.Errorf("%q has no mp4", hs[i].Headline)
		}
	}
	if aranda == nil {
		t.Fatal("Aranda's clip not found")
	}
	if aranda.Headline != "Jonathan Aranda's RBI single" || len(aranda.PlayerIDs) == 0 || aranda.PlayerIDs[0] != 666018 {
		t.Errorf("clip = %+v", aranda)
	}
	if aranda.PageURL() != "https://www.mlb.com/video/jonathan-aranda-singles-on-a-ground-ball-to-left-fielder-steven-kwan-ben" {
		t.Errorf("page = %s", aranda.PageURL())
	}
	if aranda.ThumbURL == "" || aranda.Date.IsZero() {
		t.Errorf("thumb/date missing: %+v", aranda)
	}
}
