package scheduler

import (
	"fmt"
	"time"

	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
)

// springTask predicts Rockies spring training games. Picks are kept in season
// state, not prediction_history.json, so they never move the real weights.
func (s *Scheduler) springTask(season int, r *tickResult) {
	s.settleSpringPicks(season, r)

	games, err := s.mlb.GetTodayGames()
	if err != nil {
		r.fail("spring schedule", err)
		return
	}
	for _, g := range games {
		if g.GameType != "S" && g.GameType != "E" {
			continue
		}
		if !g.IsPlayable() || g.Status == "Final" {
			continue
		}
		key := fmt.Sprintf("spring:pred:%d", g.GamePk)
		if s.done(key) {
			continue
		}
		due, missed := s.pregameWindow(g.GameDateTime, r)
		if missed {
			s.logger.Printf("missed the pregame window for spring game %d, skipping it", g.GamePk)
			s.markDone(key, "")
			continue
		}
		if !due {
			continue
		}

		post := s.buildSpringPost(g)
		uri, err := s.postOnce(key, post.Text, s.generateImage(post.HoroscopeText), "", "")
		if err != nil {
			r.fail("spring prediction", err)
			continue
		}
		if s.findPick(pickSpring, g.GamePk) == nil {
			s.state().Picks = append(s.state().Picks, pick{
				Kind: pickSpring, Season: season, GamePk: g.GamePk, Date: g.OfficialDate,
				TeamID: mlb.RockiesID, TeamName: "Colorado Rockies", Opponent: g.Opponent().Name,
				Pick: post.Prediction.Pick, WinProbability: post.Prediction.WinProbability,
				PostURI: uri, FirstPitch: g.GameDateTime,
			})
			s.saveSeasonState()
		}
		s.pollForResult(g.GameDateTime, r)
	}
}

func (s *Scheduler) buildSpringPost(g *mlb.Game) formatter.Post {
	var record *mlb.TeamRecord
	springRecord := ""
	if us := g.RockiesTeam(); us.Wins+us.Losses > 0 {
		record = &mlb.TeamRecord{
			Wins: us.Wins, Losses: us.Losses,
			WinningPercentage: float64(us.Wins) / float64(us.Wins+us.Losses),
		}
		springRecord = us.WinLossString()
	}
	_, ourStats := s.pitcherLine(g.RockiesPitcher())
	_, theirStats := s.pitcherLine(g.OpponentPitcher())

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
		IsHome:          g.IsHome,
		HoroscopeText:   horoText,
	}, weights)

	return formatter.FormatSpringGame(formatter.SpringGame{
		Opponent:      g.Opponent().Name,
		IsHome:        g.IsHome,
		GameTime:      g.FormatGameTime(),
		Venue:         g.Venue,
		SpringRecord:  springRecord,
		Prediction:    pred,
		HoroscopeText: horoText,
	})
}

func (s *Scheduler) settleSpringPicks(season int, r *tickResult) {
	st := s.state()
	results := map[string][]mlb.GameResult{}
	for i := range st.Picks {
		p := &st.Picks[i]
		if p.Kind != pickSpring || p.Season != season || p.Result != "" {
			continue
		}
		if _, ok := results[p.Date]; !ok {
			games, err := s.mlb.GetGamesSince(p.Date)
			if err != nil {
				r.fail("spring results", err)
				return
			}
			results[p.Date] = games
		}

		var gr *mlb.GameResult
		for j := range results[p.Date] {
			if results[p.Date][j].GamePk == p.GamePk {
				gr = &results[p.Date][j]
			}
		}
		if gr == nil {
			// Rained out, or never went final. Stop waiting after a couple of days.
			if r.now.Sub(p.FirstPitch) > 48*time.Hour {
				p.Result = "void"
				s.saveSeasonState()
				continue
			}
			s.pollForResult(p.FirstPitch, r)
			continue
		}

		result := "L"
		if gr.Won {
			result = "W"
		}
		p.Result = result
		text := formatter.FormatFollowUp(formatter.FollowUp{
			Outcome: "Rockies " + result,
			Score:   fmt.Sprintf("Colorado Rockies %d-%d %s", gr.RockiesScore, gr.OppScore, gr.Opponent),
			Correct: result == p.Pick,
			Record:  s.picksRecord(pickSpring, season) + " spring picks. They don't count either.",
		})
		if _, err := s.postOnce(fmt.Sprintf("spring:reply:%d", p.GamePk), text, nil, p.PostURI, p.PostURI); err != nil {
			p.Result = ""
			r.fail("spring result", err)
			continue
		}
		p.Score = fmt.Sprintf("%d-%d", gr.RockiesScore, gr.OppScore)
		s.saveSeasonState()
	}
}
