package astro

import "testing"

func TestReadMatchup(t *testing.T) {
	tests := []struct {
		batter, pitcher Sign
		edge            float64
		describe        string
	}{
		// Water douses fire; cardinal beats fixed.
		{Cancer, Leo, 0.8, "♋ Lowe's water douses ♌ Baker's fire."},
		{Leo, Cancer, -0.8, "♋ Baker's water douses ♌ Lowe's fire."},
		// Fire/earth are indifferent; mutable dodges cardinal.
		{Sagittarius, Capricorn, 0.2, "♐ Lowe's fire and ♑ Baker's earth ignore each other."},
		{Aries, Leo, 0.2, "♈ Lowe vs ♌ Baker: both fire. Someone's getting burned."},
		{Pisces, Pisces, 0, "♓ Lowe vs ♓ Baker: a Pisces mirror match. Nobody blinks."},
		// Air erodes earth; mutable vs fixed favors the fixed pitcher.
		{Gemini, Taurus, 0.4, "♊ Lowe's air erodes ♉ Baker's earth."},
	}
	for _, tt := range tests {
		m := ReadMatchup(tt.batter, tt.pitcher)
		if diff := m.Edge - tt.edge; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s vs %s: edge %.2f, want %.2f", tt.batter, tt.pitcher, m.Edge, tt.edge)
		}
		if got := m.Describe("Lowe", "Baker"); got != tt.describe {
			t.Errorf("%s vs %s:\n got %q\nwant %q", tt.batter, tt.pitcher, got, tt.describe)
		}
	}
}

// Every pairing is antisymmetric: what the batter gains, the pitcher loses.
func TestReadMatchup_Antisymmetric(t *testing.T) {
	for a := Aries; a <= Pisces; a++ {
		for b := Aries; b <= Pisces; b++ {
			ab, ba := ReadMatchup(a, b).Edge, ReadMatchup(b, a).Edge
			if ab != -ba {
				t.Errorf("%s/%s: %.2f vs %.2f", a, b, ab, ba)
			}
			if ab > 0.8 || ab < -0.8 {
				t.Errorf("%s/%s edge %.2f out of range", a, b, ab)
			}
		}
	}
}
