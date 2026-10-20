package live

import (
	"testing"

	"github.com/tlugger/rockiscope/internal/mlb"
)

// A Cancer (water) batter vs a Leo (fire) pitcher: water douses fire, so the
// batter has the stars' backing.
var waterVsFire = map[int]mlb.Person{
	1: {BirthDate: "1995-07-05"}, // Cancer
	2: {BirthDate: "1990-08-10"}, // Leo
}

func play(isTop bool, homeWP, homeWPA, leverage float64) mlb.LivePlay {
	return mlb.LivePlay{IsTop: isTop, BatterID: 1, PitcherID: 2, HomeWP: homeWP, HomeWPA: homeWPA, Leverage: leverage, IsScoringPlay: true}
}

func TestEvaluate_ForeseenPlayIsAmplified(t *testing.T) {
	// We're home, our batter (bottom half) gets a big hit the stars predicted.
	r := evaluate(play(false, 70, 15, 2), true, waterVsFire, 0)
	if !r.SignsKnown || r.Destiny <= 0 {
		t.Fatalf("destiny = %.2f, want positive", r.Destiny)
	}
	if r.Real != 70 || r.WPA != 15 || r.Cosmic <= r.Real {
		t.Errorf("stars should amplify a foreseen swing: %+v", r)
	}
	if r.PrevCosmic != 55 {
		t.Errorf("prev = %.1f, want 55 (70 before the 15-point swing)", r.PrevCosmic)
	}
}

func TestEvaluate_FlukeIsDampened(t *testing.T) {
	// We're away; their batter (bottom half) has the cosmic edge but grounds
	// into a double play that helps us. The stars call our good fortune a fluke.
	r := evaluate(play(false, 30, -12, 2), false, waterVsFire, 0)
	if r.WPA != 12 || r.Destiny >= 0 {
		t.Fatalf("wpa %.1f destiny %.2f", r.WPA, r.Destiny)
	}
	if r.Cosmic >= r.Real {
		t.Errorf("stars should doubt a fluke: real %.1f cosmic %.1f", r.Real, r.Cosmic)
	}
}

func TestEvaluate_UnknownSignsOnlyDecay(t *testing.T) {
	r := evaluate(play(true, 40, -10, 1), true, nil, 10)
	if r.SignsKnown || r.Offset != 10*offsetDecay {
		t.Errorf("offset = %.2f, want plain decay", r.Offset)
	}
}

func TestEvaluate_StarsStayHumble(t *testing.T) {
	r := evaluate(play(false, 98, 30, 5), true, waterVsFire, maxOffset)
	if r.Offset > maxOffset || r.Cosmic > 99 {
		t.Errorf("offset %.1f cosmic %.1f", r.Offset, r.Cosmic)
	}
}

func TestIsNotable(t *testing.T) {
	for _, c := range []struct {
		p    mlb.LivePlay
		want bool
	}{
		{mlb.LivePlay{IsScoringPlay: true}, true},
		{mlb.LivePlay{EventType: "home_run"}, true},
		{mlb.LivePlay{HomeWPA: -11}, true},
		{mlb.LivePlay{Leverage: 2.1}, true},
		{mlb.LivePlay{EventType: "strikeout", HomeWPA: 3, Leverage: 1.2}, false},
	} {
		if got := isNotable(c.p); got != c.want {
			t.Errorf("isNotable(%+v) = %v", c.p, got)
		}
	}
}
