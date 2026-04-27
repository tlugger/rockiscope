package mlb

import "time"

// Game represents a single Rockies game.
type Game struct {
	GamePk       int
	GameDateTime time.Time
	Status       string // "Preview", "Live", "Final"
	HomeTeam     TeamInfo
	AwayTeam     TeamInfo
	Venue        string
	IsHome       bool // true if Rockies are the home team
	GameNumber   int    // 1 or 2 for double-headers
	DoubleHeader string // "Y" (straight), "S" (split), "N" (not)
}

type TeamInfo struct {
	ID             int
	Name           string
	Wins           int
	Losses         int
	WinPct         string
	ProbablePitcher *PitcherInfo
}

type PitcherInfo struct {
	ID       int
	FullName string
}

// TeamRecord represents current season standings for the Rockies.
type TeamRecord struct {
	Wins              int
	Losses            int
	WinningPercentage float64
	RunsScored        int
	RunsAllowed       int
	RunDifferential   int
	StreakCode        string // e.g. "W3", "L2"
	HomeWins          int
	HomeLosses        int
	AwayWins          int
	AwayLosses        int
	Last10Wins        int
	Last10Losses      int
}

// PitcherStats represents season pitching stats for a single pitcher.
type PitcherStats struct {
	PlayerID      int
	FullName      string
	ERA           float64
	WHIP          float64
	Wins          int
	Losses        int
	StrikeOuts    int
	InningsPitched float64
	GamesStarted  int
}

// H2HRecord represents head-to-head record vs a specific opponent this season.
type H2HRecord struct {
	OpponentID   int
	OpponentName string
	Wins         int
	Losses       int
	GamesPlayed  int
}

// GameProvider is the interface for fetching MLB data. All business logic
// depends on this interface, making it trivially mockable in tests.
type GameProvider interface {
	GetTodayGame() (*Game, error)
	GetTodayGames() ([]*Game, error)
	GetTeamRecord() (*TeamRecord, error)
	GetPitcherStats(playerID int) (*PitcherStats, error)
	GetHeadToHead(opponentID int) (*H2HRecord, error)
	GetGamesSince(date string) ([]GameResult, error)
}
