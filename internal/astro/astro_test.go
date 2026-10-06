package astro

import (
	"strings"
	"testing"
	"time"
)

func TestSignFor(t *testing.T) {
	cases := map[string]Sign{
		"1991-07-05": Cancer, // the Rockies
		"1995-02-22": Pisces,
		"1992-01-04": Capricorn,
		"1990-12-22": Capricorn,
		"1990-12-21": Sagittarius,
		"1990-01-20": Aquarius,
		"1990-03-21": Aries,
		"1990-03-20": Pisces,
		"1990-10-23": Scorpio,
		"1990-06-21": Cancer,
		"1990-07-23": Leo,
	}
	for date, want := range cases {
		got, ok := SignFor(date)
		if !ok || got != want {
			t.Errorf("SignFor(%s) = %v, want %v", date, got, want)
		}
	}
	if _, ok := SignFor("not a date"); ok {
		t.Error("expected parse failure")
	}
}

func TestEverySignHasFullInfo(t *testing.T) {
	for s := Aries; s <= Pisces; s++ {
		if s.Name() == "" || s.Emoji() == "" || s.Trait() == "" || s.Element() == "" {
			t.Errorf("sign %d missing info", s)
		}
		if got, ok := ParseSign(s.Name()); !ok || got != s {
			t.Errorf("ParseSign(%s) round-trip failed", s.Name())
		}
		if len([]rune(s.Trait())) > 90 {
			t.Errorf("%s trait too long for a post", s.Name())
		}
	}
	if !Cancer.IsWater() || Leo.IsWater() {
		t.Error("element mismatch")
	}
}

func TestSignForDate_EveryDayOfYear(t *testing.T) {
	d := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for d.Year() == 2024 {
		if s := SignForDate(d.Month(), d.Day()); !s.valid() {
			t.Fatalf("%s produced invalid sign", d.Format("Jan 2"))
		}
		d = d.AddDate(0, 0, 1)
	}
}

func TestReadRoster(t *testing.T) {
	r := ReadRoster([]string{"1991-07-05", "1995-02-22", "1990-04-01", "1990-04-02", "garbage"})
	if r.Players != 4 {
		t.Fatalf("players = %d", r.Players)
	}
	if r.WaterShare != 0.5 {
		t.Errorf("water = %v", r.WaterShare)
	}
	if r.Score != 1.5 { // (3+3+0+0)/4
		t.Errorf("score = %v", r.Score)
	}
	if r.Dominant != "Aries" {
		t.Errorf("dominant = %s", r.Dominant)
	}
	if !strings.Contains(r.Summary(), "50% water signs") {
		t.Errorf("summary = %s", r.Summary())
	}
}

func TestReadRoster_Empty(t *testing.T) {
	if r := ReadRoster(nil); r.Players != 0 || r.Score != 0 {
		t.Errorf("got %+v", r)
	}
}

func TestBetterAndLabels(t *testing.T) {
	a := RosterReading{Score: 1.5, WaterShare: 0.2}
	b := RosterReading{Score: 1.5, WaterShare: 0.3}
	if !b.Better(a) || a.Better(b) {
		t.Error("water share should break ties")
	}
	if !(RosterReading{Score: 2}).Better(b) {
		t.Error("score should dominate")
	}
	for score, want := range map[int]string{3: "soulmates", 2: "a solid match", 1: "it's complicated", 0: "a cosmic mismatch"} {
		if got := CompatibilityLabel(score); got != want {
			t.Errorf("label(%d) = %s", score, got)
		}
	}
}
