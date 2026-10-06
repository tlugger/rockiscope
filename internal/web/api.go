package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/fsutil"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/prediction"
	"github.com/tlugger/rockiscope/internal/seasonstate"
)

// The dashboard API is strictly read-only. Every file it reads is written
// atomically by the bot (with a .bak fallback), and every file is optional:
// a fresh install renders empty states rather than errors.

// gameView is one predicted game from the perspective of the team the bot was
// picking for (the Rockies, or the adopted postseason team).
type gameView struct {
	Date           string                  `json:"date"`
	Team           string                  `json:"team"`
	Opponent       string                  `json:"opponent"`
	IsHome         bool                    `json:"isHome"`
	Predicted      string                  `json:"predicted"`
	Actual         string                  `json:"actual,omitempty"`
	TeamScore      int                     `json:"teamScore"`
	OppScore       int                     `json:"oppScore"`
	WinProbability float64                 `json:"winProbability"`
	Factors        prediction.FactorScores `json:"factors"`
	Synthetic      bool                    `json:"synthetic,omitempty"`
	Series         string                  `json:"series,omitempty"`
	SeriesResult   string                  `json:"seriesResult,omitempty"`
	PostURL        string                  `json:"postUrl,omitempty"`
}

type picksSummary struct {
	Picks   int      `json:"picks"`
	Correct int      `json:"correct"`
	Pending int      `json:"pending"`
	Wins    int      `json:"wins"`
	Losses  int      `json:"losses"`
	Adopted []string `json:"adopted,omitempty"`
}

type seasonInfo struct {
	Season        int                       `json:"season"`
	Live          bool                      `json:"live"`
	Archived      bool                      `json:"archived"`
	Summary       *prediction.SeasonSummary `json:"summary,omitempty"`
	Postseason    *picksSummary             `json:"postseason,omitempty"`
	Spring        *picksSummary             `json:"spring,omitempty"`
	HotStoveMoves int                       `json:"hotStoveMoves"`
	Champion      string                    `json:"champion,omitempty"`
}

type audit struct {
	TeamID  int                 `json:"teamId"`
	Team    string              `json:"team"`
	Reading astro.RosterReading `json:"reading"`
	Adopted bool                `json:"adopted"`
}

type hotStoveView struct {
	seasonstate.HotStoveEntry
	SignEmoji          string `json:"signEmoji,omitempty"`
	CompatibilityLabel string `json:"compatibilityLabel,omitempty"`
	PostURL            string `json:"postUrl,omitempty"`
}

type postseasonView struct {
	Adoptions    []seasonstate.Adoption `json:"adoptions"`
	Audits       []audit                `json:"audits"`
	Games        []gameView             `json:"games"`
	Champion     string                 `json:"champion,omitempty"`
	ChampionDate string                 `json:"championDate,omitempty"`
}

type seasonDetail struct {
	seasonInfo
	Phase      string              `json:"phase,omitempty"`
	Weights    *prediction.Weights `json:"weights,omitempty"`
	Regular    []gameView          `json:"regular"`
	Postseason postseasonView      `json:"postseason"`
	Spring     []gameView          `json:"spring"`
	HotStove   []hotStoveView      `json:"hotStove"`
}

type countdownView struct {
	Season     int    `json:"season"`
	OpeningDay string `json:"openingDay"`
	Opponent   string `json:"opponent,omitempty"`
	IsHome     bool   `json:"isHome"`
	Days       int    `json:"days"`
	SpringDays int    `json:"springDays"`
}

type statusView struct {
	Phase      string                    `json:"phase,omitempty"`
	Season     int                       `json:"season,omitempty"`
	UpdatedAt  string                    `json:"updatedAt,omitempty"`
	Today      string                    `json:"today"`
	Countdown  *countdownView            `json:"countdown,omitempty"`
	Adoption   *seasonstate.Adoption     `json:"adoption,omitempty"`
	LastPick   *gameView                 `json:"lastPick,omitempty"`
	Postseason *picksSummary             `json:"postseason,omitempty"`
	Spring     *picksSummary             `json:"spring,omitempty"`
	Champion   string                    `json:"champion,omitempty"`
	LatestMove *hotStoveView             `json:"latestMove,omitempty"`
	Rockies    *prediction.SeasonSummary `json:"rockies,omitempty"`
}

// snapshot is everything on disk, read once per request.
type snapshot struct {
	live     *prediction.PredictionHistory
	state    *seasonstate.State
	archived map[int]bool
}

func (s *Server) load() snapshot {
	snap := snapshot{archived: map[int]bool{}}

	if s.dataDir != "" {
		if h, err := prediction.LoadHistory(s.dataDir); err == nil {
			snap.live = h
		} else {
			s.logger.Printf("dashboard: prediction history unreadable: %v", err)
		}
		st, _, err := seasonstate.Read(s.dataDir)
		if err != nil && !os.IsNotExist(err) {
			s.logger.Printf("dashboard: season state unreadable: %v", err)
		}
		snap.state = st
		entries, _ := os.ReadDir(filepath.Join(s.dataDir, prediction.ArchiveDirName))
		for _, e := range entries {
			if y, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
				snap.archived[y] = true
			}
		}
	}
	if snap.live == nil {
		snap.live = &prediction.PredictionHistory{}
	}
	if snap.state == nil {
		snap.state = seasonstate.New()
	}
	return snap
}

func (s *Server) readArchive(season int) (*prediction.PredictionHistory, *prediction.SeasonSummary) {
	dir := filepath.Join(s.dataDir, prediction.ArchiveDirName, strconv.Itoa(season))
	var hist *prediction.PredictionHistory
	if data, _, err := fsutil.ReadFileWithFallback(filepath.Join(dir, "prediction_history.json"), validJSON); err == nil {
		var h prediction.PredictionHistory
		if json.Unmarshal(data, &h) == nil {
			hist = &h
		}
	}
	var sum *prediction.SeasonSummary
	if data, _, err := fsutil.ReadFileWithFallback(filepath.Join(dir, "summary.json"), validJSON); err == nil {
		var ss prediction.SeasonSummary
		if json.Unmarshal(data, &ss) == nil {
			sum = &ss
		}
	}
	if sum == nil && hist != nil {
		ss := prediction.SummarizeSeason(hist.Predictions, season, hist.Current)
		sum = &ss
	}
	return hist, sum
}

func validJSON(b []byte) error {
	var v interface{}
	return json.Unmarshal(b, &v)
}

func (snap snapshot) seasons() []int {
	set := map[int]bool{}
	for _, p := range snap.live.Predictions {
		if y := prediction.SeasonOf(p.Date); y > 0 {
			set[y] = true
		}
	}
	for y := range snap.archived {
		set[y] = true
	}
	st := snap.state
	for _, p := range st.Picks {
		set[p.Season] = true
	}
	for _, a := range st.Adoptions {
		set[a.Season] = true
	}
	for _, e := range st.HotStove {
		set[e.Season] = true
	}
	if st.Status.Season > 0 {
		set[st.Status.Season] = true
	}
	var out []int
	for y := range set {
		if y > 0 {
			out = append(out, y)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

func (snap snapshot) liveSeason() int {
	if snap.state.Status.Season > 0 {
		return snap.state.Status.Season
	}
	if ys := snap.seasons(); len(ys) > 0 {
		return ys[0]
	}
	return 0
}

func (snap snapshot) liveRecords(season int) []prediction.PredictionRecord {
	var out []prediction.PredictionRecord
	for _, p := range snap.live.Predictions {
		if prediction.SeasonOf(p.Date) == season {
			out = append(out, p)
		}
	}
	return out
}

func (snap snapshot) picks(kind string, season int) []seasonstate.Pick {
	var out []seasonstate.Pick
	for _, p := range snap.state.Picks {
		if p.Kind == kind && p.Season == season {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].FirstPitch.Before(out[j].FirstPitch) })
	return out
}

func summarizePicks(picks []seasonstate.Pick) *picksSummary {
	if len(picks) == 0 {
		return nil
	}
	ps := &picksSummary{}
	for _, p := range picks {
		switch p.Result {
		case "W", "L":
			ps.Picks++
			if p.Result == p.Pick {
				ps.Correct++
			}
			if p.Result == "W" {
				ps.Wins++
			} else {
				ps.Losses++
			}
		case "":
			ps.Pending++
		}
	}
	return ps
}

func (s *Server) info(snap snapshot, season int) seasonInfo {
	in := seasonInfo{Season: season, Live: season == snap.liveSeason(), Archived: snap.archived[season]}

	if recs := snap.liveRecords(season); len(recs) > 0 {
		sum := prediction.SummarizeSeason(recs, season, snap.live.Current)
		in.Summary = &sum
	} else if in.Archived {
		_, in.Summary = s.readArchive(season)
	}
	if in.Summary != nil && in.Summary.Predictions == 0 && in.Summary.RockiesWins+in.Summary.RockiesLosses == 0 {
		in.Summary = nil
	}

	in.Postseason = summarizePicks(snap.picks(seasonstate.PickPostseason, season))
	for _, a := range snap.state.Adoptions {
		if a.Season == season {
			if in.Postseason == nil {
				in.Postseason = &picksSummary{}
			}
			in.Postseason.Adopted = append(in.Postseason.Adopted, a.TeamName)
		}
	}
	in.Spring = summarizePicks(snap.picks(seasonstate.PickSpring, season))
	for _, e := range snap.state.HotStove {
		if e.Season == season {
			in.HotStoveMoves++
		}
	}
	in.Champion = snap.state.Champions[strconv.Itoa(season)]
	return in
}

// postURL turns at://did/app.bsky.feed.post/rkey into a bsky.app link.
func postURL(uri string) string {
	parts := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if !strings.HasPrefix(uri, "at://") || len(parts) != 3 || parts[1] != "app.bsky.feed.post" || !strings.HasPrefix(parts[0], "did:") {
		return ""
	}
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", parts[0], parts[2])
}

func regularGames(recs []prediction.PredictionRecord) []gameView {
	out := []gameView{}
	for _, p := range recs {
		if p.Opponent == "Off Day" || p.Predicted == "N/A" {
			continue
		}
		out = append(out, gameView{
			Date: p.Date, Team: "Colorado Rockies", Opponent: p.Opponent, IsHome: p.IsHome,
			Predicted: p.Predicted, Actual: p.Actual, TeamScore: p.RockiesScore, OppScore: p.OppScore,
			WinProbability: p.WinProbability, Factors: p.Factors, Synthetic: p.Synthetic,
			PostURL: postURL(p.PostURI),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func pickGames(picks []seasonstate.Pick) []gameView {
	out := []gameView{}
	for _, p := range picks {
		if p.Result == "void" {
			continue
		}
		g := gameView{
			Date: p.Date, Team: p.TeamName, Opponent: p.Opponent, IsHome: p.IsHome,
			Predicted: p.Pick, TeamScore: p.TeamScore, OppScore: p.OppScore,
			WinProbability: p.WinProbability, Factors: p.Factors,
			Series: p.Series, SeriesResult: p.SeriesResult, PostURL: postURL(p.PostURI),
		}
		if p.Result == "W" || p.Result == "L" {
			g.Actual = p.Result
		}
		out = append(out, g)
	}
	return out
}

func hotStoveViews(entries []seasonstate.HotStoveEntry, season int) []hotStoveView {
	out := []hotStoveView{}
	for _, e := range entries {
		if season != 0 && e.Season != season {
			continue
		}
		v := hotStoveView{HotStoveEntry: e, PostURL: postURL(e.PostURI)}
		if sign, ok := astro.ParseSign(e.Sign); ok {
			v.SignEmoji = sign.Emoji()
		}
		if e.Compatibility >= 0 && e.Arriving {
			v.CompatibilityLabel = astro.CompatibilityLabel(e.Compatibility)
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].ID > out[j].ID
	})
	return out
}

func (s *Server) detail(snap snapshot, season int) seasonDetail {
	d := seasonDetail{seasonInfo: s.info(snap, season)}
	if d.Live {
		d.Phase = snap.state.Status.Phase
	}

	if recs := snap.liveRecords(season); len(recs) > 0 {
		d.Regular = regularGames(recs)
		w := snap.live.Current
		d.Weights = &w
	} else if d.Archived {
		if hist, _ := s.readArchive(season); hist != nil {
			d.Regular = regularGames(hist.Predictions)
			w := hist.Current
			d.Weights = &w
		}
	} else {
		d.Regular = []gameView{}
	}

	st := snap.state
	ps := postseasonView{Adoptions: []seasonstate.Adoption{}, Audits: []audit{}}
	adopted := map[int]bool{}
	for _, a := range st.Adoptions {
		if a.Season == season {
			ps.Adoptions = append(ps.Adoptions, a)
			adopted[a.TeamID] = true
		}
	}
	prefix := strconv.Itoa(season) + ":"
	for key, reading := range st.Readings {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
		if err != nil {
			continue
		}
		name := mlb.TeamNames[id]
		if name == "" {
			name = fmt.Sprintf("Team %d", id)
		}
		ps.Audits = append(ps.Audits, audit{TeamID: id, Team: name, Reading: reading, Adopted: adopted[id]})
	}
	sort.SliceStable(ps.Audits, func(i, j int) bool {
		if ps.Audits[i].Reading.Better(ps.Audits[j].Reading) != ps.Audits[j].Reading.Better(ps.Audits[i].Reading) {
			return ps.Audits[i].Reading.Better(ps.Audits[j].Reading)
		}
		return ps.Audits[i].TeamID < ps.Audits[j].TeamID
	})
	ps.Games = pickGames(snap.picks(seasonstate.PickPostseason, season))
	ps.Champion = st.Champions[strconv.Itoa(season)]
	ps.ChampionDate = st.ChampionDates[strconv.Itoa(season)]
	d.Postseason = ps

	d.Spring = pickGames(snap.picks(seasonstate.PickSpring, season))
	d.HotStove = hotStoveViews(st.HotStove, season)
	return d
}

func (s *Server) status(snap snapshot) statusView {
	st := snap.state
	today := s.now().In(mlb.DenverLocation()).Format("2006-01-02")
	v := statusView{Phase: st.Status.Phase, Season: st.Status.Season, UpdatedAt: st.Status.UpdatedAt, Today: today}
	season := v.Season

	// Count down to the next season that hasn't started.
	var next string
	for key, dates := range st.SeasonDates {
		if dates.RegularStart > today && (next == "" || key < next) {
			next = key
		}
	}
	if next != "" {
		dates := st.SeasonDates[next]
		od := st.OpeningDays[next]
		if od.Date == "" {
			od.Date = dates.RegularStart
		}
		if days := daysBetween(today, od.Date); days > 0 {
			y, _ := strconv.Atoi(next)
			v.Countdown = &countdownView{
				Season: y, OpeningDay: od.Date, Opponent: od.Opponent, IsHome: od.IsHome,
				Days: days, SpringDays: daysBetween(today, dates.SpringStart),
			}
		}
	}

	if season > 0 {
		for i := len(st.Adoptions) - 1; i >= 0; i-- {
			if st.Adoptions[i].Season == season {
				a := st.Adoptions[i]
				v.Adoption = &a
				break
			}
		}
		post := snap.picks(seasonstate.PickPostseason, season)
		v.Postseason = summarizePicks(post)
		if games := pickGames(post); len(games) > 0 {
			last := games[len(games)-1]
			v.LastPick = &last
		}
		v.Spring = summarizePicks(snap.picks(seasonstate.PickSpring, season))
		v.Champion = st.Champions[strconv.Itoa(season)]
		if recs := snap.liveRecords(season); len(recs) > 0 {
			sum := prediction.SummarizeSeason(recs, season, snap.live.Current)
			v.Rockies = &sum
		}
	}
	if moves := hotStoveViews(st.HotStove, 0); len(moves) > 0 {
		v.LatestMove = &moves[0]
	}
	return v
}

func daysBetween(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encoding response", http.StatusInternalServerError)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.status(s.load()))
}

func (s *Server) handleSeasons(w http.ResponseWriter, r *http.Request) {
	snap := s.load()
	out := []seasonInfo{}
	for _, y := range snap.seasons() {
		out = append(out, s.info(snap, y))
	}
	writeJSON(w, out)
}

func (s *Server) handleSeason(w http.ResponseWriter, r *http.Request) {
	season, err := strconv.Atoi(r.PathValue("year"))
	if err != nil || season < 1900 || season > 3000 {
		http.Error(w, "invalid season", http.StatusBadRequest)
		return
	}
	snap := s.load()
	found := false
	for _, y := range snap.seasons() {
		if y == season {
			found = true
		}
	}
	if !found {
		http.Error(w, "no data for that season", http.StatusNotFound)
		return
	}
	writeJSON(w, s.detail(snap, season))
}
