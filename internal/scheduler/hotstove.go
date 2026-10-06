package scheduler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tlugger/rockiscope/internal/astro"
	"github.com/tlugger/rockiscope/internal/formatter"
	"github.com/tlugger/rockiscope/internal/mlb"
	"github.com/tlugger/rockiscope/internal/seasonstate"
)

// Transaction checks happen at these Denver hours.
var txnCheckHours = []int{10, 17}

const (
	// maxTxnPostsPerCheck caps individual posts per check; the rest share a digest.
	maxTxnPostsPerCheck = 3
	// txnLookbackDays bounds catch-up after downtime. Older news stays old.
	txnLookbackDays = 7
)

// Moves worth a post. Minor-league signings, assignments and IL shuffles are noise.
var notableTxn = map[string]bool{"TR": true, "SFA": true, "CLW": true, "DES": true, "REL": true, "DFA": true, "R5": true}

// Moves where the player is leaving (or on the way out).
var departingTxn = map[string]bool{"DES": true, "REL": true, "DFA": true}

func isNotableTxn(t mlb.Transaction) bool {
	if !notableTxn[t.TypeCode] {
		return false
	}
	return !(t.TypeCode == "SFA" && strings.Contains(strings.ToLower(t.Description), "minor league contract"))
}

func isArriving(t mlb.Transaction) bool {
	return t.ToTeamID == mlb.RockiesID && !departingTxn[t.TypeCode]
}

// hotStoveTask posts new Rockies roster moves, twice a day, and stays silent
// when nothing happens.
func (s *Scheduler) hotStoveTask(season int, r *tickResult) {
	st := s.state()
	today := s.today()
	if st.HotStoveSince == "" {
		st.HotStoveSince = today
		s.saveSeasonState()
	}

	slot := -1
	for _, h := range txnCheckHours {
		if at := clockOn(r.now, h, 0); r.now.Before(at) {
			r.wakeAt(at)
			break
		}
		slot = h
	}
	r.wakeAt(s.nextClock(r.now, txnCheckHours[0], 0))
	if slot < 0 {
		if !s.ignoreTimeGates {
			return
		}
		slot = 0
	}
	checkKey := fmt.Sprintf("txn-check:%s:%d", today, slot)
	if s.done(checkKey) || !s.chatterAllowed(r) {
		return
	}

	since := r.now.AddDate(0, 0, -txnLookbackDays).Format("2006-01-02")
	if st.HotStoveSince > since {
		since = st.HotStoveSince
	}
	txns, err := s.season.GetTransactions(since, today)
	if err != nil {
		r.fail("transactions", err)
		return
	}

	var fresh []mlb.Transaction
	byDesc := map[string]int{}
	for _, t := range txns {
		id := fmt.Sprint(t.ID)
		if _, seen := st.TxnSeen[id]; seen {
			continue
		}
		if t.Date < since || !isNotableTxn(t) {
			st.TxnSeen[id] = t.Date
			continue
		}
		// A trade is listed once per player under the same description. Post
		// it once, charting the player who's arriving.
		if i, dup := byDesc[t.Description]; dup {
			if isArriving(t) && !isArriving(fresh[i]) {
				st.TxnSeen[fmt.Sprint(fresh[i].ID)] = fresh[i].Date
				fresh[i] = t
			} else {
				st.TxnSeen[id] = t.Date
			}
			continue
		}
		byDesc[t.Description] = len(fresh)
		fresh = append(fresh, t)
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		if fresh[i].Date != fresh[j].Date {
			return fresh[i].Date < fresh[j].Date
		}
		return fresh[i].ID < fresh[j].ID
	})
	s.saveSeasonState()

	if len(fresh) > 0 {
		s.logger.Printf("hot stove: %d new Rockies moves", len(fresh))
		if !s.postTransactions(season, fresh, today, slot, r) {
			return
		}
	}
	s.markDone(checkKey, "")
}

// postTransactions posts the first few moves individually and rolls the rest
// into a digest. It returns false if anything failed (to retry later).
func (s *Scheduler) postTransactions(season int, fresh []mlb.Transaction, today string, slot int, r *tickResult) bool {
	st := s.state()
	birthDates := map[int]string{}
	var ids []int
	for i, t := range fresh {
		if i < maxTxnPostsPerCheck && t.PersonID != 0 {
			ids = append(ids, t.PersonID)
		}
	}
	if people, err := s.season.GetPeople(ids); err != nil {
		s.logger.Printf("warning: birth dates unavailable, posting without charts: %v", err)
	} else {
		for _, p := range people {
			birthDates[p.ID] = p.BirthDate
		}
	}

	var digest []string
	var digestIDs []mlb.Transaction
	for i, t := range fresh {
		if i >= maxTxnPostsPerCheck {
			digest = append(digest, t.Description)
			digestIDs = append(digestIDs, t)
			continue
		}
		text := formatter.FormatTransaction(formatter.TransactionPost{
			TypeCode:    t.TypeCode,
			Description: t.Description,
			PlayerName:  t.PersonName,
			BirthDate:   birthDates[t.PersonID],
			Arriving:    isArriving(t),
		})
		uri, err := s.postOnce(fmt.Sprintf("txn:%d", t.ID), text, nil, "", "")
		if err != nil {
			r.fail("transaction post", err)
			return false
		}
		st.TxnSeen[fmt.Sprint(t.ID)] = t.Date
		s.logHotStove(season, t, birthDates[t.PersonID], uri, false)
		s.saveSeasonState()
	}

	if len(digest) > 0 {
		key := fmt.Sprintf("txn-digest:%s:%d", today, slot)
		uri, err := s.postOnce(key, formatter.FormatTransactionDigest(digest), nil, "", "")
		if err != nil {
			r.fail("transaction digest", err)
			return false
		}
		for _, t := range digestIDs {
			st.TxnSeen[fmt.Sprint(t.ID)] = t.Date
			s.logHotStove(season, t, "", uri, true)
		}
		s.saveSeasonState()
	}
	return true
}

// logHotStove keeps a permanent record of each posted move for the dashboard.
// Unlike TxnSeen it is never pruned (a busy winter is ~50 entries).
func (s *Scheduler) logHotStove(season int, t mlb.Transaction, birthDate, uri string, digest bool) {
	st := s.state()
	for _, e := range st.HotStove {
		if e.ID == t.ID {
			return
		}
	}
	e := seasonstate.HotStoveEntry{
		Season: season, Date: t.Date, ID: t.ID, TypeCode: t.TypeCode, TypeDesc: t.TypeDesc,
		Description: t.Description, Player: t.PersonName, Arriving: isArriving(t),
		Compatibility: -1, PostURI: uri, Digest: digest,
	}
	if sign, ok := astro.SignFor(birthDate); ok {
		e.Sign = sign.Name()
		e.Compatibility = sign.Compatibility()
	}
	st.HotStove = append(st.HotStove, e)
}
