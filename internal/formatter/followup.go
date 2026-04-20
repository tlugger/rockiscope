package formatter

import "fmt"

type FollowUp struct {
	Outcome    string
	Score     string
	Correct   bool
	Record    string
	Opponent  string
	PostURI   string
}

func FormatFollowUp(f FollowUp) string {
	if f.Correct {
		return fmt.Sprintf("The cosmos don't miss. ✨\n%s %s. %s",
			f.Outcome, f.Score, f.Record)
	}

	return fmt.Sprintf("Called it. (I wasn't.)\n%s %s. %s",
		f.Outcome, f.Score, f.Record)
}