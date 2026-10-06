package scheduler

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// bracket is what we can infer about the postseason from its games.
type bracket struct {
	teams      map[int]string
	eliminated map[int]bool
	champion   int
	games      []mlb.PostseasonGame
}

// isRealTeam filters out bracket placeholders like "Lower Seed League Champion".
func isRealTeam(id int) bool { return id >= 108 && id <= 160 }

// analyzeBracket counts series wins to find who's been eliminated and who won
// it all. It relies only on final scores, never on schedule placeholders.
func analyzeBracket(games []mlb.PostseasonGame) bracket {
	b := bracket{teams: map[int]string{}, eliminated: map[int]bool{}, games: games}
	type seriesKey struct {
		gameType string
		lo, hi   int
	}
	wins := map[seriesKey]map[int]int{}
	length := map[seriesKey]int{}

	for _, g := range games {
		if !isRealTeam(g.Away.ID) || !isRealTeam(g.Home.ID) {
			continue
		}
		b.teams[g.Away.ID] = g.Away.Name
		b.teams[g.Home.ID] = g.Home.Name
		if g.Status != "Final" {
			continue
		}
		lo, hi := g.Away.ID, g.Home.ID
		if lo > hi {
			lo, hi = hi, lo
		}
		k := seriesKey{g.GameType, lo, hi}
		if wins[k] == nil {
			wins[k] = map[int]int{}
		}
		if g.GamesInSeries > length[k] {
			length[k] = g.GamesInSeries
		}
		switch {
		case g.Home.IsWinner:
			wins[k][g.Home.ID]++
		case g.Away.IsWinner:
			wins[k][g.Away.ID]++
		}
	}

	for k, w := range wins {
		need := length[k]/2 + 1
		for team, n := range w {
			if n < need {
				continue
			}
			loser := k.lo
			if loser == team {
				loser = k.hi
			}
			b.eliminated[loser] = true
			if k.gameType == "W" {
				b.champion = team
			}
		}
	}
	return b
}

func (b bracket) alive() []int {
	var ids []int
	for id := range b.teams {
		if !b.eliminated[id] {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}

func (b bracket) game(gamePk int) *mlb.PostseasonGame {
	for i := range b.games {
		if b.games[i].GamePk == gamePk {
			return &b.games[i]
		}
	}
	return nil
}

func (s *Scheduler) currentAdoption(season int) *adoption {
	st := s.state()
	for i := len(st.Adoptions) - 1; i >= 0; i-- {
		a := &st.Adoptions[i]
		if a.Season == season && a.EliminatedOn == "" {
			return a
		}
	}
	return nil
}

func (s *Scheduler) adoptions(season int) []adoption {
	var out []adoption
	for _, a := range s.state().Adoptions {
		if a.Season == season {
			out = append(out, a)
		}
	}
	return out
}

func (s *Scheduler) postseasonNeedsWrapUp(season int) bool {
	_, crowned := s.state().Champions[seasonKey(season)]
	return !crowned && len(s.adoptions(season)) > 0
}

// postseasonTask is one pass of October: settle finished games, handle
// eliminations, (re)adopt, crown a champion, and post today's predictions.
func (s *Scheduler) postseasonTask(season int, r *tickResult) {
	games, err := s.season.GetPostseasonGames(season)
	if err != nil {
		r.fail("postseason schedule", err)
		return
	}
	b := analyzeBracket(games)

	s.settlePostseasonPicks(season, b, r)

	cur := s.currentAdoption(season)
	if cur != nil && b.eliminated[cur.TeamID] {
		s.logger.Printf("adopted team %s eliminated", cur.TeamName)
		cur.EliminatedOn = s.today()
		s.saveSeasonState()
		cur = nil
	}

	if b.champion != 0 {
		s.crownChampion(season, b, r)
		return
	}

	if cur == nil {
		if cur = s.adoptTeam(season, b, r); cur == nil {
			return
		}
	}
	s.predictAdoptedGames(season, *cur, b, r)
}

// teamReading reads (and caches) a team's roster birth charts.
func (s *Scheduler) teamReading(season, teamID int) (astro.RosterReading, error) {
	key := fmt.Sprintf("%d:%d", season, teamID)
	if r, ok := s.state().Readings[key]; ok {
		return r, nil
	}
	roster, err := s.season.GetRoster(teamID)
	if err != nil {
		return astro.RosterReading{}, err
	}
	dates := make([]string, 0, len(roster))
	for _, p := range roster {
		dates = append(dates, p.BirthDate)
	}
	reading := astro.ReadRoster(dates)
	if reading.Players == 0 {
		return reading, fmt.Errorf("no birth dates on team %d's roster", teamID)
	}
	s.state().Readings[key] = reading
	s.saveSeasonState()
	return reading, nil
}

// adoptTeam picks the most Cancer-compatible roster still alive and announces it.
func (s *Scheduler) adoptTeam(season int, b bracket, r *tickResult) *adoption {
	alive := b.alive()
	if len(alive) == 0 || !s.chatterAllowed(r) {
		return nil
	}

	best, audited := 0, 0
	var bestReading astro.RosterReading
	for _, id := range alive {
		reading, err := s.teamReading(season, id)
		if err != nil {
			s.logger.Printf("warning: couldn't read %s's birth charts: %v", b.teams[id], err)
			continue
		}
		audited++
		// Loyalty first: if the Rockies are somehow alive, they're the pick.
		if id == mlb.RockiesID || (best != mlb.RockiesID && (best == 0 || reading.Better(bestReading))) {
			best, bestReading = id, reading
		}
	}
	if best == 0 {
		r.fail("adoption", errors.New("no playoff rosters could be read"))
		return nil
	}

	in := formatter.Adoption{
		TeamName:       b.teams[best],
		Reading:        bestReading,
		TeamsAudited:   audited,
		AdoptingItself: best == mlb.RockiesID,
	}
	if prev := s.adoptions(season); len(prev) > 0 {
		in.PreviousTeam = prev[len(prev)-1].TeamName
	} else if rec, err := s.season.GetTeamRecordFor(mlb.RockiesID, season); err == nil {
		in.RockiesRecord = fmt.Sprintf("%d-%d", rec.Wins, rec.Losses)
	}

	key := fmt.Sprintf("ps:%d:adopt:%d", season, best)
	if _, err := s.postOnce(key, formatter.FormatAdoption(in), nil, "", ""); err != nil {
		r.fail("adoption", err)
		return nil
	}

	st := s.state()
	st.Adoptions = append(st.Adoptions, adoption{
		Season: season, TeamID: best, TeamName: b.teams[best],
		AdoptedOn: s.today(), Reading: bestReading,
	})
	s.saveSeasonState()
	s.logger.Printf("adopted the %s (compatibility %.2f)", b.teams[best], bestReading.Score)
	return &st.Adoptions[len(st.Adoptions)-1]
}

func (s *Scheduler) predictAdoptedGames(season int, a adoption, b bracket, r *tickResult) {
	today := s.today()
	for _, g := range b.games {
		if !g.Involves(a.TeamID) || g.Status == "Final" || g.StartTimeTBD {
			continue
		}
		if g.DetailedState == "Postponed" || g.DetailedState == "Cancelled" {
			continue
		}
		if g.OfficialDate > today {
			r.wakeAt(g.GameDateTime.Add(-time.Hour))
			continue
		}
		if g.OfficialDate < today {
			continue
		}
		key := fmt.Sprintf("ps:pred:%d", g.GamePk)
		if s.done(key) {
			continue
		}
		due, missed := s.pregameWindow(g.GameDateTime, r)
		if missed {
			s.logger.Printf("missed the pregame window for game %d, skipping it", g.GamePk)
			s.markDone(key, "")
			continue
		}
		if !due {
			continue
		}

		post := s.buildAdoptedGamePost(season, a, g)
		uri, err := s.postOnce(key, post.Text, s.generateImage(post.HoroscopeText), "", "")
		if err != nil {
			r.fail("adopted game prediction", err)
			continue
		}
		team, opp, isHome := g.Side(a.TeamID)
		if s.findPick(pickPostseason, g.GamePk) == nil {
			s.state().Picks = append(s.state().Picks, pick{
				Kind: pickPostseason, Season: season, GamePk: g.GamePk, Date: g.OfficialDate,
				TeamID: team.ID, TeamName: team.Name, Opponent: opp.Name, IsHome: isHome,
				Pick: post.Prediction.Pick, WinProbability: post.Prediction.WinProbability,
				Factors: post.Prediction.FactorScores(), Series: g.SeriesShort,
				TeamStarter: post.TeamStarter, OppStarter: post.OppStarter,
				PostURI: uri, FirstPitch: g.GameDateTime,
			})
			s.saveSeasonState()
		}
		s.pollForResult(g.GameDateTime, r)
	}
}

func (s *Scheduler) buildAdoptedGamePost(season int, a adoption, g mlb.PostseasonGame) formatter.Post {
	team, opp, isHome := g.Side(a.TeamID)

	record, err := s.season.GetTeamRecordFor(a.TeamID, season)
	if err != nil {
		s.logger.Printf("warning: %s record unavailable: %v", a.TeamName, err)
	}
	pitcherText, ourStats := s.pitcherLine(team.ProbablePitcher)
	_, theirStats := s.pitcherLine(opp.ProbablePitcher)

	horoText := ""
	if horo, err := s.horoscope.GetDailyHoroscope(); err != nil {
		s.logger.Printf("warning: could not get horoscope: %v", err)
	} else if horo != nil {
		horoText = horo.Text
	}

	weights := prediction.DefaultWeights()
	if s.predHistory != nil {
		weights = s.predHistory.Current
	}
	pred := prediction.Predict(prediction.Input{
		Record:          record,
		RockiesPitcher:  ourStats,
		OpponentPitcher: theirStats,
		IsHome:          isHome,
		HoroscopeText:   horoText,
	}, weights)

	post := formatter.FormatAdoptedGame(formatter.AdoptedGame{
		TeamName:      team.Name,
		Opponent:      opp.Name,
		IsHome:        isHome,
		SeriesShort:   g.SeriesShort,
		SeriesResult:  g.SeriesResult,
		GameTime:      formatClock(g.GameDateTime),
		Venue:         g.Venue,
		Pitcher:       pitcherText,
		Prediction:    pred,
		HoroscopeText: horoText,
	})
	post.TeamStarter = prediction.NewStarter(team.ProbablePitcher, ourStats)
	post.OppStarter = prediction.NewStarter(opp.ProbablePitcher, theirStats)
	return post
}

// settlePostseasonPicks replies to each prediction once its game is final.
func (s *Scheduler) settlePostseasonPicks(season int, b bracket, r *tickResult) {
	st := s.state()
	for i := range st.Picks {
		p := &st.Picks[i]
		if p.Kind != pickPostseason || p.Season != season || p.Result != "" {
			continue
		}
		g := b.game(p.GamePk)
		if g == nil {
			continue
		}
		if g.DetailedState == "Postponed" || g.DetailedState == "Cancelled" {
			p.Result = "void"
			s.saveSeasonState()
			continue
		}
		team, opp, _ := g.Side(p.TeamID)
		if g.Status != "Final" || (!team.IsWinner && !opp.IsWinner) {
			s.pollForResult(g.GameDateTime, r)
			continue
		}

		result := "L"
		if team.IsWinner {
			result = "W"
		}
		// Count this game in the running record before announcing it.
		p.Result = result
		text := formatter.FormatAdoptedResult(formatter.AdoptedResult{
			TeamName:     p.TeamName,
			Opponent:     p.Opponent,
			Won:          result == "W",
			Correct:      result == p.Pick,
			Score:        fmt.Sprintf("%d-%d", team.Score, opp.Score),
			SeriesResult: g.SeriesResult,
			PicksRecord:  s.picksRecord(pickPostseason, season),
			Seed:         p.GamePk,
		})
		if _, err := s.postOnce(fmt.Sprintf("ps:reply:%d", p.GamePk), text, nil, p.PostURI, p.PostURI); err != nil {
			p.Result = ""
			r.fail("adopted game result", err)
			continue
		}
		p.Score = fmt.Sprintf("%d-%d", team.Score, opp.Score)
		p.TeamScore, p.OppScore = team.Score, opp.Score
		p.SeriesResult = g.SeriesResult
		if p.Series == "" {
			p.Series = g.SeriesShort
		}
		s.saveSeasonState()
	}
}

func (s *Scheduler) crownChampion(season int, b bracket, r *tickResult) {
	st := s.state()
	key := fmt.Sprintf("ps:%d:champion", season)
	name := b.teams[b.champion]

	in := formatter.Champion{Champion: name, PicksRecord: s.picksRecord(pickPostseason, season)}
	for _, a := range s.adoptions(season) {
		if a.TeamID == b.champion {
			in.AdoptedWon, in.AdoptedOn = true, a.AdoptedOn
		}
		in.Adopted = append(in.Adopted, a.TeamName)
	}
	if len(in.Adopted) == 0 {
		in.PicksRecord = ""
	}
	if _, err := s.postOnce(key, formatter.FormatChampion(in), nil, "", ""); err != nil {
		r.fail("champion post", err)
		return
	}

	st.Champions[seasonKey(season)] = name
	st.ChampionDates[seasonKey(season)] = s.today()
	if st.HotStoveSince == "" || st.HotStoveSince < s.today() {
		st.HotStoveSince = s.today()
	}
	s.saveSeasonState()
	s.logger.Printf("%s won the %d World Series; switching to hot stove mode", name, season)
}
