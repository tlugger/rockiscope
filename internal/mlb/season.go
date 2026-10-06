package mlb

import (
	"fmt"
	"strings"
	"time"
)

// SeasonDates are MLB's calendar milestones for one season, all "2006-01-02".
type SeasonDates struct {
	Season       string `json:"seasonId"`
	SpringStart  string `json:"springStartDate"`
	RegularStart string `json:"regularSeasonStartDate"`
	RegularEnd   string `json:"regularSeasonEndDate"`
	PostStart    string `json:"postSeasonStartDate"`
	PostEnd      string `json:"postSeasonEndDate"`
}

// PostseasonTeam is one side of a postseason game.
type PostseasonTeam struct {
	ID              int
	Name            string
	Score           int
	IsWinner        bool
	ProbablePitcher *PitcherInfo
}

// PostseasonGame is a single playoff game with its series context.
type PostseasonGame struct {
	GamePk            int
	GameType          string // F wild card, D division, L league, W World Series
	OfficialDate      string
	GameDateTime      time.Time
	StartTimeTBD      bool
	Status            string // abstractGameState
	DetailedState     string
	Venue             string
	SeriesDescription string // "NL Division Series"
	SeriesShort       string // "NLDS Game 3"
	SeriesResult      string // "ATL leads 2-1", as of this game
	SeriesGameNumber  int
	GamesInSeries     int
	Away, Home        PostseasonTeam
}

// Involves reports whether teamID plays in this game.
func (g PostseasonGame) Involves(teamID int) bool {
	return g.Away.ID == teamID || g.Home.ID == teamID
}

// Side returns (team, opponent, isHome) from teamID's perspective.
func (g PostseasonGame) Side(teamID int) (PostseasonTeam, PostseasonTeam, bool) {
	if g.Home.ID == teamID {
		return g.Home, g.Away, true
	}
	return g.Away, g.Home, false
}

// Person is a player with the one stat that matters here: their birthday.
type Person struct {
	ID        int
	FullName  string
	BirthDate string
	Position  string
}

// Transaction is a roster move.
type Transaction struct {
	ID          int
	Date        string
	TypeCode    string
	TypeDesc    string
	Description string
	PersonID    int
	PersonName  string
	ToTeamID    int
	FromTeamID  int
}

// SeasonProvider fetches the off-the-field calendar and roster data used
// outside the regular season.
type SeasonProvider interface {
	GetSeasonDates(year int) (*SeasonDates, error)
	GetPostseasonGames(season int) ([]PostseasonGame, error)
	GetRoster(teamID int) ([]Person, error)
	GetPeople(ids []int) ([]Person, error)
	GetTransactions(startDate, endDate string) ([]Transaction, error)
	GetTeamRecordFor(teamID, season int) (*TeamRecord, error)
	GetRockiesGamesBetween(startDate, endDate, gameType string) ([]*Game, error)
	GetSeasonResultsFor(season int) ([]GameResult, error)
}

func (c *Client) GetSeasonDates(year int) (*SeasonDates, error) {
	url := fmt.Sprintf("%s/seasons/%d?sportId=1", c.base(), year)
	var resp struct {
		Seasons []SeasonDates `json:"seasons"`
	}
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching season dates: %w", err)
	}
	if len(resp.Seasons) == 0 || resp.Seasons[0].RegularStart == "" {
		return nil, fmt.Errorf("no season dates published for %d", year)
	}
	return &resp.Seasons[0], nil
}

func (c *Client) GetPostseasonGames(season int) ([]PostseasonGame, error) {
	url := fmt.Sprintf("%s/schedule?sportId=1&season=%d&gameType=F,D,L,W&hydrate=probablePitcher,seriesStatus", c.base(), season)
	var resp scheduleResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching postseason schedule: %w", err)
	}
	return parsePostseasonGames(resp), nil
}

func parsePostseasonGames(resp scheduleResponse) []PostseasonGame {
	var games []PostseasonGame
	for _, d := range resp.Dates {
		for _, g := range d.Games {
			t, err := time.Parse(time.RFC3339, g.GameDate)
			if err != nil {
				continue
			}
			pg := PostseasonGame{
				GamePk:            g.GamePk,
				GameType:          g.GameType,
				OfficialDate:      g.OfficialDate,
				GameDateTime:      t,
				StartTimeTBD:      g.Status.StartTimeTBD,
				Status:            g.Status.AbstractGameState,
				DetailedState:     g.Status.DetailedState,
				Venue:             g.Venue.Name,
				SeriesDescription: g.SeriesDescription,
				SeriesShort:       g.SeriesStatus.ShortDescription,
				SeriesResult:      g.SeriesStatus.Result,
				SeriesGameNumber:  g.SeriesGameNumber,
				GamesInSeries:     g.GamesInSeries,
				Away:              postseasonTeam(g.Teams.Away),
				Home:              postseasonTeam(g.Teams.Home),
			}
			if pg.OfficialDate == "" {
				pg.OfficialDate = t.In(denverLoc()).Format("2006-01-02")
			}
			if pg.SeriesShort == "" && pg.SeriesDescription != "" {
				pg.SeriesShort = fmt.Sprintf("%s Game %d", pg.SeriesDescription, pg.SeriesGameNumber)
			}
			games = append(games, pg)
		}
	}
	return games
}

func postseasonTeam(t scheduleTeam) PostseasonTeam {
	pt := PostseasonTeam{ID: t.Team.ID, Name: t.Team.Name, Score: t.Score, IsWinner: t.IsWinner}
	if t.ProbablePitcher.ID != 0 {
		pt.ProbablePitcher = &PitcherInfo{ID: t.ProbablePitcher.ID, FullName: t.ProbablePitcher.FullName}
	}
	return pt
}

type personJSON struct {
	ID              int    `json:"id"`
	FullName        string `json:"fullName"`
	BirthDate       string `json:"birthDate"`
	PrimaryPosition struct {
		Abbreviation string `json:"abbreviation"`
	} `json:"primaryPosition"`
}

func (p personJSON) toPerson() Person {
	return Person{ID: p.ID, FullName: p.FullName, BirthDate: p.BirthDate, Position: p.PrimaryPosition.Abbreviation}
}

func (c *Client) base() string {
	if c.apiBase != "" {
		return c.apiBase
	}
	return baseURL
}

// GetRoster returns a team's active roster (the playoff roster in October).
func (c *Client) GetRoster(teamID int) ([]Person, error) {
	url := fmt.Sprintf("%s/teams/%d/roster?rosterType=active&hydrate=person", c.base(), teamID)
	var resp struct {
		Roster []struct {
			Person personJSON `json:"person"`
		} `json:"roster"`
	}
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching roster for team %d: %w", teamID, err)
	}
	people := make([]Person, 0, len(resp.Roster))
	for _, r := range resp.Roster {
		people = append(people, r.Person.toPerson())
	}
	return people, nil
}

// GetPeople looks up several players in one request.
func (c *Client) GetPeople(ids []int) ([]Person, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = fmt.Sprint(id)
	}
	url := fmt.Sprintf("%s/people?personIds=%s", c.base(), strings.Join(strs, ","))
	var resp struct {
		People []personJSON `json:"people"`
	}
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching people: %w", err)
	}
	people := make([]Person, 0, len(resp.People))
	for _, p := range resp.People {
		people = append(people, p.toPerson())
	}
	return people, nil
}

// GetTransactions returns Rockies roster moves dated within [startDate, endDate].
func (c *Client) GetTransactions(startDate, endDate string) ([]Transaction, error) {
	url := fmt.Sprintf("%s/transactions?teamId=%d&startDate=%s&endDate=%s", c.base(), c.teamID, startDate, endDate)
	var resp struct {
		Transactions []struct {
			ID     int `json:"id"`
			Person struct {
				ID       int    `json:"id"`
				FullName string `json:"fullName"`
			} `json:"person"`
			ToTeam struct {
				ID int `json:"id"`
			} `json:"toTeam"`
			FromTeam struct {
				ID int `json:"id"`
			} `json:"fromTeam"`
			Date        string `json:"date"`
			TypeCode    string `json:"typeCode"`
			TypeDesc    string `json:"typeDesc"`
			Description string `json:"description"`
		} `json:"transactions"`
	}
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching transactions: %w", err)
	}
	txns := make([]Transaction, 0, len(resp.Transactions))
	for _, t := range resp.Transactions {
		txns = append(txns, Transaction{
			ID: t.ID, Date: t.Date, TypeCode: t.TypeCode, TypeDesc: t.TypeDesc,
			Description: strings.Join(strings.Fields(t.Description), " "),
			PersonID:    t.Person.ID, PersonName: t.Person.FullName,
			ToTeamID: t.ToTeam.ID, FromTeamID: t.FromTeam.ID,
		})
	}
	return txns, nil
}

// GetTeamRecordFor returns any team's regular-season record for a given season.
func (c *Client) GetTeamRecordFor(teamID, season int) (*TeamRecord, error) {
	url := fmt.Sprintf("%s/standings?leagueId=103,104&season=%d", c.base(), season)
	var resp standingsResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching standings: %w", err)
	}
	return parseTeamRecordFor(resp, teamID)
}

// GetRockiesGamesBetween lists Rockies games of a type (e.g. "R", "S") in a date range.
func (c *Client) GetRockiesGamesBetween(startDate, endDate, gameType string) ([]*Game, error) {
	url := fmt.Sprintf("%s/schedule?sportId=1&teamId=%d&startDate=%s&endDate=%s&gameType=%s",
		c.base(), c.teamID, startDate, endDate, gameType)
	var resp scheduleResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, fmt.Errorf("fetching schedule: %w", err)
	}
	var games []*Game
	for _, d := range resp.Dates {
		for _, g := range d.Games {
			game, err := c.parseGame(g)
			if err != nil {
				continue
			}
			games = append(games, game)
		}
	}
	return games, nil
}
