package mlb

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LiveGame is the slow-moving part of a game in progress: its status and
// who's playing (with birthdays, naturally).
type LiveGame struct {
	GamePk        int
	Status        string // abstractGameState: "Preview", "Live", "Final"
	DetailedState string
	Away, Home    LiveTeam
	Players       map[int]Person
}

type LiveTeam struct {
	ID     int
	Name   string
	Abbrev string
}

// LivePlay is one plate appearance with its win-probability impact.
type LivePlay struct {
	AtBatIndex    int
	Inning        int
	IsTop         bool
	Event         string // "Home Run"
	EventType     string // "home_run"
	Description   string
	RBI           int
	AwayScore     int
	HomeScore     int
	IsComplete    bool
	IsScoringPlay bool
	EndTime       time.Time
	BatterID      int
	BatterName    string
	PitcherID     int
	PitcherName   string
	// PlayIDs are the play's pitch/event UUIDs, last one last. Highlight clips
	// are keyed by the last pitch's playId.
	PlayIDs []string
	// LaunchSpeed (mph) and Distance (ft) of the ball in play, 0 if none.
	LaunchSpeed float64
	Distance    float64
	// HomeWP is the home team's win probability after the play, 0-100;
	// HomeWPA is how much this play moved it.
	HomeWP   float64
	HomeWPA  float64
	Leverage float64
}

// Highlight is one video clip from a game's content feed.
type Highlight struct {
	GUID      string // matches a LivePlay's last playId; empty for non-play clips
	Slug      string
	Headline  string
	Blurb     string
	Date      time.Time
	MP4URL    string
	ThumbURL  string
	PlayerIDs []int
}

// PageURL is the clip's page on mlb.com.
func (h Highlight) PageURL() string {
	if h.Slug == "" {
		return h.MP4URL
	}
	return "https://www.mlb.com/video/" + h.Slug
}

// LiveProvider fetches what the in-game watcher needs.
type LiveProvider interface {
	GetLiveGame(gamePk int) (*LiveGame, error)
	GetLivePlays(gamePk int) ([]LivePlay, error)
	GetHighlights(gamePk int) ([]Highlight, error)
}

// The feed and win-probability endpoints are hundreds of KB; the fields filter
// trims them to what we read (a 780KB feed becomes ~4KB, plays ~70KB).
const (
	liveGameFields = "gameData,status,abstractGameState,detailedState,players,id,fullName,birthDate,teams,away,home,name,abbreviation"
	livePlayFields = "atBatIndex,result,event,eventType,description,rbi,awayScore,homeScore," +
		"about,inning,isTopInning,isComplete,isScoringPlay,endTime," +
		"matchup,batter,pitcher,id,fullName,playEvents,playId,hitData,launchSpeed,totalDistance," +
		"homeTeamWinProbability,homeTeamWinProbabilityAdded,leverageIndex"
)

func (c *Client) liveBase() string {
	return strings.Replace(c.base(), "/api/v1", "/api/v1.1", 1)
}

func (c *Client) GetLiveGame(gamePk int) (*LiveGame, error) {
	url := fmt.Sprintf("%s/game/%d/feed/live?fields=%s", c.liveBase(), gamePk, liveGameFields)
	var resp liveFeedResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching live game %d: %w", gamePk, err)
	}
	return resp.toLiveGame(gamePk), nil
}

func (c *Client) GetLivePlays(gamePk int) ([]LivePlay, error) {
	url := fmt.Sprintf("%s/game/%d/winProbability?fields=%s", c.base(), gamePk, livePlayFields)
	var resp []livePlayJSON
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching plays for game %d: %w", gamePk, err)
	}
	return parseLivePlays(resp), nil
}

func (c *Client) GetHighlights(gamePk int) ([]Highlight, error) {
	url := fmt.Sprintf("%s/game/%d/content", c.base(), gamePk)
	var resp contentResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching highlights for game %d: %w", gamePk, err)
	}
	return parseHighlights(resp), nil
}

type liveFeedResponse struct {
	GameData struct {
		Status struct {
			AbstractGameState string `json:"abstractGameState"`
			DetailedState     string `json:"detailedState"`
		} `json:"status"`
		Teams struct {
			Away liveTeamJSON `json:"away"`
			Home liveTeamJSON `json:"home"`
		} `json:"teams"`
		Players map[string]personJSON `json:"players"`
	} `json:"gameData"`
}

type liveTeamJSON struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}

func (t liveTeamJSON) toLiveTeam() LiveTeam {
	return LiveTeam{ID: t.ID, Name: t.Name, Abbrev: t.Abbreviation}
}

func (r liveFeedResponse) toLiveGame(gamePk int) *LiveGame {
	gd := r.GameData
	g := &LiveGame{
		GamePk:        gamePk,
		Status:        gd.Status.AbstractGameState,
		DetailedState: gd.Status.DetailedState,
		Away:          gd.Teams.Away.toLiveTeam(),
		Home:          gd.Teams.Home.toLiveTeam(),
		Players:       make(map[int]Person, len(gd.Players)),
	}
	for _, p := range gd.Players {
		g.Players[p.ID] = p.toPerson()
	}
	return g
}

type livePlayJSON struct {
	AtBatIndex int `json:"atBatIndex"`
	Result     struct {
		Event       string `json:"event"`
		EventType   string `json:"eventType"`
		Description string `json:"description"`
		RBI         int    `json:"rbi"`
		AwayScore   int    `json:"awayScore"`
		HomeScore   int    `json:"homeScore"`
	} `json:"result"`
	About struct {
		Inning        int    `json:"inning"`
		IsTopInning   bool   `json:"isTopInning"`
		IsComplete    bool   `json:"isComplete"`
		IsScoringPlay bool   `json:"isScoringPlay"`
		EndTime       string `json:"endTime"`
	} `json:"about"`
	Matchup struct {
		Batter  personJSON `json:"batter"`
		Pitcher personJSON `json:"pitcher"`
	} `json:"matchup"`
	PlayEvents []struct {
		PlayID  string `json:"playId"`
		HitData *struct {
			LaunchSpeed   float64 `json:"launchSpeed"`
			TotalDistance float64 `json:"totalDistance"`
		} `json:"hitData"`
	} `json:"playEvents"`
	HomeWP   float64 `json:"homeTeamWinProbability"`
	HomeWPA  float64 `json:"homeTeamWinProbabilityAdded"`
	Leverage float64 `json:"leverageIndex"`
}

func parseLivePlays(raw []livePlayJSON) []LivePlay {
	plays := make([]LivePlay, 0, len(raw))
	for _, r := range raw {
		p := LivePlay{
			AtBatIndex:    r.AtBatIndex,
			Inning:        r.About.Inning,
			IsTop:         r.About.IsTopInning,
			Event:         r.Result.Event,
			EventType:     r.Result.EventType,
			Description:   r.Result.Description,
			RBI:           r.Result.RBI,
			AwayScore:     r.Result.AwayScore,
			HomeScore:     r.Result.HomeScore,
			IsComplete:    r.About.IsComplete,
			IsScoringPlay: r.About.IsScoringPlay,
			BatterID:      r.Matchup.Batter.ID,
			BatterName:    r.Matchup.Batter.FullName,
			PitcherID:     r.Matchup.Pitcher.ID,
			PitcherName:   r.Matchup.Pitcher.FullName,
			HomeWP:        r.HomeWP,
			HomeWPA:       r.HomeWPA,
			Leverage:      r.Leverage,
		}
		if t, err := time.Parse(time.RFC3339, r.About.EndTime); err == nil {
			p.EndTime = t
		}
		for _, e := range r.PlayEvents {
			if e.PlayID != "" {
				p.PlayIDs = append(p.PlayIDs, e.PlayID)
			}
			if e.HitData != nil {
				p.LaunchSpeed, p.Distance = e.HitData.LaunchSpeed, e.HitData.TotalDistance
			}
		}
		plays = append(plays, p)
	}
	return plays
}

type contentResponse struct {
	Highlights struct {
		Highlights struct {
			Items []struct {
				Type        string `json:"type"`
				GUID        string `json:"guid"`
				Slug        string `json:"slug"`
				Headline    string `json:"headline"`
				Blurb       string `json:"blurb"`
				Date        string `json:"date"`
				KeywordsAll []struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"keywordsAll"`
				Playbacks []struct {
					Name string `json:"name"`
					URL  string `json:"url"`
				} `json:"playbacks"`
				Image struct {
					Cuts []struct {
						Width int    `json:"width"`
						Src   string `json:"src"`
					} `json:"cuts"`
				} `json:"image"`
			} `json:"items"`
		} `json:"highlights"`
	} `json:"highlights"`
}

// thumbWidth is the image cut used for link-card thumbnails; Bluesky caps
// thumbs at 1MB and 640px is plenty.
const thumbWidth = 640

func parseHighlights(resp contentResponse) []Highlight {
	var out []Highlight
	for _, it := range resp.Highlights.Highlights.Items {
		if it.Type != "" && it.Type != "video" {
			continue
		}
		h := Highlight{GUID: it.GUID, Slug: it.Slug, Headline: strings.TrimSpace(it.Headline), Blurb: it.Blurb}
		if t, err := time.Parse(time.RFC3339, it.Date); err == nil {
			h.Date = t
		}
		for _, p := range it.Playbacks {
			if p.Name == "mp4Avc" {
				h.MP4URL = p.URL
			}
		}
		if h.MP4URL == "" {
			continue
		}
		for _, cut := range it.Image.Cuts {
			if h.ThumbURL == "" || cut.Width == thumbWidth {
				h.ThumbURL = cut.Src
			}
		}
		for _, k := range it.KeywordsAll {
			if k.Type != "player_id" {
				continue
			}
			if id, err := strconv.Atoi(k.Value); err == nil {
				h.PlayerIDs = append(h.PlayerIDs, id)
			}
		}
		out = append(out, h)
	}
	return out
}
