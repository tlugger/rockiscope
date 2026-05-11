package mlb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL    = "https://statsapi.mlb.com/api/v1"
	RockiesID  = 115
	NLLeagueID = 104
)

// Client implements GameProvider using the MLB Stats API.
type Client struct {
	httpClient *http.Client
	teamID     int
	now        func() time.Time
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		httpClient: httpClient,
		teamID:     RockiesID,
		now:        time.Now,
	}
}

func (c *Client) GetTodayGame() (*Game, error) {
	today := c.now().In(denverLoc()).Format("2006-01-02")
	url := fmt.Sprintf("%s/schedule?sportId=1&teamId=%d&date=%s&hydrate=probablePitcher", baseURL, c.teamID, today)
	return c.getGameFromURL(url)
}

func (c *Client) GetTodayGames() ([]*Game, error) {
	today := c.now().In(denverLoc()).Format("2006-01-02")
	url := fmt.Sprintf("%s/schedule?sportId=1&teamId=%d&date=%s&hydrate=probablePitcher", baseURL, c.teamID, today)
	return c.getGamesFromURL(url)
}

func (c *Client) GetTeamRecord() (*TeamRecord, error) {
	season := c.now().Year()
	url := fmt.Sprintf("%s/standings?leagueId=%d&season=%d", baseURL, NLLeagueID, season)
	return c.getTeamRecordFromURL(url)
}

func (c *Client) GetPitcherStats(playerID int) (*PitcherStats, error) {
	url := fmt.Sprintf("%s/people/%d?hydrate=stats(group=[pitching],type=[season])", baseURL, playerID)
	return c.getPitcherStatsFromURL(url)
}

func (c *Client) GetHeadToHead(opponentID int) (*H2HRecord, error) {
	season := c.now().Year()
	url := fmt.Sprintf("%s/schedule?sportId=1&teamId=%d&season=%d&gameType=R", baseURL, c.teamID, season)
	return c.getH2HFromURL(url, opponentID)
}

type GameResult struct {
	GamePk       int    `json:"gamePk"`
	Date        string `json:"date"`
	OpponentID  int    `json:"opponentId"`
	Opponent    string `json:"opponent"`
	IsHome      bool   `json:"isHome"`
	RockiesScore int   `json:"rockiesScore"`
	OppScore    int    `json:"oppScore"`
	Won         bool   `json:"won"`
	Status     string `json:"status"`
}

func (c *Client) GetGamesSince(date string) ([]GameResult, error) {
	url := fmt.Sprintf("%s/schedule?sportId=1&teamId=%d&startDate=%s&endDate=%s&hydrate=linescore",
		baseURL, c.teamID, date, date)
	var resp scheduleResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching games: %w", err)
	}
	return c.parseGameResults(resp), nil
}

// GetSeasonResults returns every completed regular-season Rockies game for the current season.
func (c *Client) GetSeasonResults() ([]GameResult, error) {
	season := c.now().Year()
	url := fmt.Sprintf("%s/schedule?sportId=1&teamId=%d&season=%d&gameType=R&hydrate=linescore",
		baseURL, c.teamID, season)
	var resp scheduleResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching season schedule: %w", err)
	}
	return c.parseGameResults(resp), nil
}

func (c *Client) parseGameResults(resp scheduleResponse) []GameResult {
	var results []GameResult
	for _, d := range resp.Dates {
		for _, g := range d.Games {
			if g.Status.AbstractGameState != "Final" {
				continue
			}

			gameTime, err := time.Parse(time.RFC3339Nano, g.GameDate)
			if err != nil {
				gameTime, err = time.Parse("2006-01-02T15:04:05Z", g.GameDate)
				if err != nil {
					continue
				}
			}

			var gr GameResult
			gr.GamePk = g.GamePk
			gr.Date = gameTime.Format("2006-01-02")
			gr.Status = g.Status.AbstractGameState

			if g.Teams.Away.Team.ID == c.teamID {
				gr.OpponentID = g.Teams.Home.Team.ID
				gr.Opponent = g.Teams.Home.Team.Name
				gr.IsHome = false
				gr.RockiesScore = g.Teams.Away.Score
				gr.OppScore = g.Teams.Home.Score
				gr.Won = g.Teams.Away.Score > g.Teams.Home.Score
			} else {
				gr.OpponentID = g.Teams.Away.Team.ID
				gr.Opponent = g.Teams.Away.Team.Name
				gr.IsHome = true
				gr.RockiesScore = g.Teams.Home.Score
				gr.OppScore = g.Teams.Away.Score
				gr.Won = g.Teams.Home.Score > g.Teams.Away.Score
			}

			results = append(results, gr)
		}
	}
	return results
}

// URL-parameterized methods — used by both production code and tests.

func (c *Client) getGameFromURL(url string) (*Game, error) {
	var resp scheduleResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching schedule: %w", err)
	}
	if len(resp.Dates) == 0 || len(resp.Dates[0].Games) == 0 {
		return nil, nil
	}
	return c.parseGame(resp.Dates[0].Games[0])
}

func (c *Client) getGamesFromURL(url string) ([]*Game, error) {
	var resp scheduleResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching schedule: %w", err)
	}
	if len(resp.Dates) == 0 || len(resp.Dates[0].Games) == 0 {
		return nil, nil
	}
	var games []*Game
	for _, g := range resp.Dates[0].Games {
		game, err := c.parseGame(g)
		if err != nil {
			return nil, fmt.Errorf("parsing game: %w", err)
		}
		games = append(games, game)
	}
	return games, nil
}

func (c *Client) getTeamRecordFromURL(url string) (*TeamRecord, error) {
	var resp standingsResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching standings: %w", err)
	}
	return c.parseTeamRecord(resp)
}

func (c *Client) getPitcherStatsFromURL(url string) (*PitcherStats, error) {
	var resp playerResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching pitcher stats: %w", err)
	}
	return c.parsePlayerStats(resp)
}

func (c *Client) getH2HFromURL(url string, opponentID int) (*H2HRecord, error) {
	var resp scheduleResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching season schedule: %w", err)
	}
	return c.parseH2H(resp, opponentID), nil
}

// Parse helpers — pure transformations from API response types to domain types.

func (c *Client) parseGame(g scheduleGame) (*Game, error) {
	gameTime, err := time.Parse(time.RFC3339Nano, g.GameDate)
	if err != nil {
		gameTime, err = time.Parse("2006-01-02T15:04:05Z", g.GameDate)
		if err != nil {
			return nil, fmt.Errorf("parsing game time %q: %w", g.GameDate, err)
		}
	}

	isHome := g.Teams.Home.Team.ID == c.teamID

	game := &Game{
		GamePk:        g.GamePk,
		GameDateTime:  gameTime,
		Status:        g.Status.AbstractGameState,
		DetailedState: g.Status.DetailedState,
		Reason:        g.Status.Reason,
		Venue:         g.Venue.Name,
		IsHome:        isHome,
		GameNumber:    g.GameNumber,
		DoubleHeader:  g.DoubleHeader,
		HomeTeam: TeamInfo{
			ID:     g.Teams.Home.Team.ID,
			Name:   g.Teams.Home.Team.Name,
			Wins:   g.Teams.Home.LeagueRecord.Wins,
			Losses: g.Teams.Home.LeagueRecord.Losses,
			WinPct: g.Teams.Home.LeagueRecord.Pct,
		},
		AwayTeam: TeamInfo{
			ID:     g.Teams.Away.Team.ID,
			Name:   g.Teams.Away.Team.Name,
			Wins:   g.Teams.Away.LeagueRecord.Wins,
			Losses: g.Teams.Away.LeagueRecord.Losses,
			WinPct: g.Teams.Away.LeagueRecord.Pct,
		},
	}

	if pp := g.Teams.Home.ProbablePitcher; pp.ID != 0 {
		game.HomeTeam.ProbablePitcher = &PitcherInfo{ID: pp.ID, FullName: pp.FullName}
	}
	if pp := g.Teams.Away.ProbablePitcher; pp.ID != 0 {
		game.AwayTeam.ProbablePitcher = &PitcherInfo{ID: pp.ID, FullName: pp.FullName}
	}

	return game, nil
}

func (c *Client) parseTeamRecord(resp standingsResponse) (*TeamRecord, error) {
	for _, record := range resp.Records {
		for _, team := range record.TeamRecords {
			if team.Team.ID != c.teamID {
				continue
			}
			winPct, _ := strconv.ParseFloat(team.WinningPercentage, 64)
			rec := &TeamRecord{
				Wins:              team.Wins,
				Losses:            team.Losses,
				WinningPercentage: winPct,
				RunsScored:        team.RunsScored,
				RunsAllowed:       team.RunsAllowed,
				RunDifferential:   team.RunDifferential,
				StreakCode:        team.Streak.StreakCode,
			}
			for _, split := range team.Records.SplitRecords {
				switch split.Type {
				case "home":
					rec.HomeWins = split.Wins
					rec.HomeLosses = split.Losses
				case "away":
					rec.AwayWins = split.Wins
					rec.AwayLosses = split.Losses
				case "lastTen":
					rec.Last10Wins = split.Wins
					rec.Last10Losses = split.Losses
				}
			}
			return rec, nil
		}
	}
	return nil, fmt.Errorf("team %d not found in standings", c.teamID)
}

func (c *Client) parsePlayerStats(resp playerResponse) (*PitcherStats, error) {
	if len(resp.People) == 0 {
		return nil, fmt.Errorf("player not found")
	}
	p := resp.People[0]
	stats := &PitcherStats{
		PlayerID: p.ID,
		FullName: p.FullName,
	}
	for _, statGroup := range p.Stats {
		if statGroup.Group.DisplayName != "pitching" {
			continue
		}
		for _, split := range statGroup.Splits {
			s := split.Stat
			stats.ERA, _ = strconv.ParseFloat(s.ERA, 64)
			stats.WHIP, _ = strconv.ParseFloat(s.WHIP, 64)
			stats.Wins = s.Wins
			stats.Losses = s.Losses
			stats.StrikeOuts = s.StrikeOuts
			stats.InningsPitched, _ = strconv.ParseFloat(s.InningsPitched, 64)
			stats.GamesStarted = s.GamesStarted
			return stats, nil
		}
	}
	return stats, nil
}

func (c *Client) parseH2H(resp scheduleResponse, opponentID int) *H2HRecord {
	h2h := &H2HRecord{OpponentID: opponentID}
	for _, date := range resp.Dates {
		for _, g := range date.Games {
			if g.Status.AbstractGameState != "Final" {
				continue
			}
			var rockiesScore, opponentScore int
			isHome := g.Teams.Home.Team.ID == c.teamID
			isAway := g.Teams.Away.Team.ID == c.teamID

			if isHome && g.Teams.Away.Team.ID == opponentID {
				if h2h.OpponentName == "" {
					h2h.OpponentName = g.Teams.Away.Team.Name
				}
				rockiesScore = g.Teams.Home.Score
				opponentScore = g.Teams.Away.Score
			} else if isAway && g.Teams.Home.Team.ID == opponentID {
				if h2h.OpponentName == "" {
					h2h.OpponentName = g.Teams.Home.Team.Name
				}
				rockiesScore = g.Teams.Away.Score
				opponentScore = g.Teams.Home.Score
			} else {
				continue
			}
			h2h.GamesPlayed++
			if rockiesScore > opponentScore {
				h2h.Wins++
			} else {
				h2h.Losses++
			}
		}
	}
	return h2h
}

func (c *Client) getJSON(url string, v interface{}) error {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d for %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func denverLoc() *time.Location {
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		return time.FixedZone("MST", -7*60*60)
	}
	return loc
}

// DenverLocation returns the America/Denver timezone.
func DenverLocation() *time.Location {
	return denverLoc()
}

// Game convenience methods.

func (g *Game) Opponent() TeamInfo {
	if g.IsHome {
		return g.AwayTeam
	}
	return g.HomeTeam
}

func (g *Game) RockiesTeam() TeamInfo {
	if g.IsHome {
		return g.HomeTeam
	}
	return g.AwayTeam
}

func (g *Game) GameTimeMST() time.Time {
	return g.GameDateTime.In(denverLoc())
}

func (g *Game) FormatGameTime() string {
	t := g.GameTimeMST()
	zone, _ := t.Zone()
	hour := t.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	ampm := "AM"
	if t.Hour() >= 12 {
		ampm = "PM"
	}
	return fmt.Sprintf("%d:%02d %s %s", hour, t.Minute(), ampm, zone)
}

func (g *Game) OpponentID() int    { return g.Opponent().ID }
func (g *Game) RockiesPitcher() *PitcherInfo {
	if g.IsHome {
		return g.HomeTeam.ProbablePitcher
	}
	return g.AwayTeam.ProbablePitcher
}
func (g *Game) OpponentPitcher() *PitcherInfo {
	if g.IsHome {
		return g.AwayTeam.ProbablePitcher
	}
	return g.HomeTeam.ProbablePitcher
}
func (g *Game) HasProbablePitchers() bool {
	return g.HomeTeam.ProbablePitcher != nil && g.AwayTeam.ProbablePitcher != nil
}
func (g *Game) IsUpcoming(now ...time.Time) bool {
	var t time.Time
	if len(now) > 0 {
		t = now[0]
	} else {
		t = time.Now()
	}
	return strings.EqualFold(g.Status, "Preview") || g.GameDateTime.After(t)
}
func (t TeamInfo) WinLossString() string {
	return fmt.Sprintf("%d-%d", t.Wins, t.Losses)
}

// API response types.

type scheduleResponse struct {
	Dates []struct {
		Games []scheduleGame `json:"games"`
	} `json:"dates"`
}

type scheduleGame struct {
	GamePk     int    `json:"gamePk"`
	GameDate   string `json:"gameDate"`
	GameNumber int    `json:"gameNumber"`
	DoubleHeader string `json:"doubleheader"`
	Status     struct {
		AbstractGameState string `json:"abstractGameState"`
		DetailedState     string `json:"detailedState"`
		Reason            string `json:"reason"`
	} `json:"status"`
	Teams  struct {
		Away scheduleTeam `json:"away"`
		Home scheduleTeam `json:"home"`
	} `json:"teams"`
	Venue  struct {
		Name string `json:"name"`
	} `json:"venue"`
}

type scheduleTeam struct {
	Score int `json:"score"`
	Team  struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	LeagueRecord struct {
		Wins   int    `json:"wins"`
		Losses int    `json:"losses"`
		Pct    string `json:"pct"`
	} `json:"leagueRecord"`
	ProbablePitcher struct {
		ID       int    `json:"id"`
		FullName string `json:"fullName"`
	} `json:"probablePitcher"`
}

type standingsResponse struct {
	Records []struct {
		TeamRecords []standingsTeam `json:"teamRecords"`
	} `json:"records"`
}

type standingsTeam struct {
	Team struct {
		ID int `json:"id"`
	} `json:"team"`
	Wins              int    `json:"wins"`
	Losses            int    `json:"losses"`
	WinningPercentage string `json:"winningPercentage"`
	RunsScored        int    `json:"runsScored"`
	RunsAllowed       int    `json:"runsAllowed"`
	RunDifferential   int    `json:"runDifferential"`
	Streak            struct {
		StreakCode string `json:"streakCode"`
	} `json:"streak"`
	Records struct {
		SplitRecords []struct {
			Wins   int    `json:"wins"`
			Losses int    `json:"losses"`
			Type   string `json:"type"`
		} `json:"splitRecords"`
	} `json:"records"`
}

type playerResponse struct {
	People []struct {
		ID       int    `json:"id"`
		FullName string `json:"fullName"`
		Stats    []struct {
			Group struct {
				DisplayName string `json:"displayName"`
			} `json:"group"`
			Splits []struct {
				Stat struct {
					ERA            string `json:"era"`
					WHIP           string `json:"whip"`
					Wins           int    `json:"wins"`
					Losses         int    `json:"losses"`
					StrikeOuts     int    `json:"strikeOuts"`
					InningsPitched string `json:"inningsPitched"`
					GamesStarted   int    `json:"gamesStarted"`
				} `json:"stat"`
			} `json:"splits"`
		} `json:"stats"`
	} `json:"people"`
}
