package scheduler

import (
	"github.com/tlugger/rockiscope/internal/live"
	"github.com/tlugger/rockiscope/internal/mlb"
)

// LiveThreads runs in-game threads (live.Manager): the scheduler registers
// each pregame post, then waits for the thread before posting the result.
type LiveThreads interface {
	Register(t live.Thread)
	WrapUp(gamePk int) (live.WrapUp, bool)
	Finish(gamePk int)
}

func (s *Scheduler) liveWrapUp(gamePk int) (live.WrapUp, bool) {
	if s.live == nil {
		return live.WrapUp{}, true
	}
	return s.live.WrapUp(gamePk)
}

func (s *Scheduler) liveFinish(gamePk int) {
	if s.live != nil {
		s.live.Finish(gamePk)
	}
}

// registerRegularLive hands a posted Rockies game to the live watcher.
func (s *Scheduler) registerRegularLive(game *mlb.Game) {
	if s.live == nil || s.predHistory == nil || game.Status == "Final" {
		return
	}
	for _, p := range s.predHistory.Predictions {
		if p.GamePK != game.GamePk || p.PostURI == "" || p.Actual != "" {
			continue
		}
		s.live.Register(live.Thread{
			GamePk:     game.GamePk,
			TeamID:     mlb.RockiesID,
			TeamName:   game.RockiesTeam().Name,
			FirstPitch: game.GameDateTime,
			PregameWP:  p.WinProbability,
			RootURI:    p.PostURI,
		})
		return
	}
}

// registerPostseasonLive hands a posted adopted-team game to the live watcher.
func (s *Scheduler) registerPostseasonLive(p *pick) {
	if s.live == nil || p == nil || p.PostURI == "" || p.Result != "" {
		return
	}
	s.live.Register(live.Thread{
		GamePk:     p.GamePk,
		TeamID:     p.TeamID,
		TeamName:   p.TeamName,
		Label:      p.Series,
		FirstPitch: p.FirstPitch,
		PregameWP:  p.WinProbability,
		RootURI:    p.PostURI,
	})
}
