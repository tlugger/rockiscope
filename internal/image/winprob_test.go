package image

import (
	"os"
	"testing"
)

func TestWinProbChart(t *testing.T) {
	var points []WPPoint
	for inning := 1; inning <= 9; inning++ {
		for _, top := range []bool{true, false} {
			for i := 0; i < 4; i++ {
				real := 50 + float64(inning*4)
				points = append(points, WPPoint{Inning: inning, IsTop: top, Real: real, Cosmic: real - 8})
			}
		}
	}

	pngBytes, w, h, err := WinProbChart("Rays @ Guardians · ALCS Game 5", points)
	if err != nil {
		t.Fatal(err)
	}
	if w != 1200 || h != 675 {
		t.Errorf("size = %dx%d", w, h)
	}
	if len(pngBytes) < 1000 || len(pngBytes) > 1_000_000 {
		t.Errorf("PNG is %d bytes; Bluesky's limit is 1MB", len(pngBytes))
	}
	if os.Getenv("SAVE_CHART") != "" {
		os.WriteFile(os.Getenv("SAVE_CHART"), pngBytes, 0644)
	}
}

func TestWinProbChart_TooFewPoints(t *testing.T) {
	if _, _, _, err := WinProbChart("x", []WPPoint{{Real: 50}}); err == nil {
		t.Error("expected an error for a single point")
	}
}
