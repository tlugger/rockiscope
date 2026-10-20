package image

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	chartWidth  = 1200
	chartHeight = 675
)

var (
	scienceCol = color.RGBA{R: 140, G: 200, B: 255, A: 255}
	starsCol   = color.RGBA{R: 255, G: 205, B: 90, A: 255}
)

// WPPoint is one play's win probability (0-100) for the team being followed.
type WPPoint struct {
	Inning int
	IsTop  bool
	Real   float64
	Cosmic float64
}

// WinProbChart draws a game's real (MLB) win probability against the stars'
// version, same night-sky look as the horoscope card.
func WinProbChart(title string, points []WPPoint) ([]byte, int, int, error) {
	if len(points) < 2 {
		return nil, 0, 0, fmt.Errorf("need at least 2 points, got %d", len(points))
	}
	fontRegular, err := truetype.Parse(goregular.TTF)
	if err != nil {
		return nil, 0, 0, err
	}
	fontBold, err := truetype.Parse(gobold.TTF)
	if err != nil {
		return nil, 0, 0, err
	}
	titleFace := truetype.NewFace(fontBold, &truetype.Options{Size: 30})
	labelFace := truetype.NewFace(fontRegular, &truetype.Options{Size: 20})

	dc := gg.NewContext(chartWidth, chartHeight)
	for y := 0; y < chartHeight; y++ {
		t := float64(y) / chartHeight
		dc.SetRGB(lerp(float64(bgTop.R), float64(bgBottom.R), t)/255,
			lerp(float64(bgTop.G), float64(bgBottom.G), t)/255,
			lerp(float64(bgTop.B), float64(bgBottom.B), t)/255)
		dc.DrawRectangle(0, float64(y), chartWidth, 1)
		dc.Fill()
	}
	drawChartStars(dc, title)

	const left, right, top, bottom = 90.0, chartWidth - 50.0, 130.0, chartHeight - 80.0
	x := func(i int) float64 { return left + (right-left)*float64(i)/float64(len(points)-1) }
	y := func(wp float64) float64 { return bottom - (bottom-top)*wp/100 }

	dc.SetFontFace(titleFace)
	dc.SetColor(accentCol)
	dc.DrawString(title, left, 60)

	// Legend
	dc.SetFontFace(labelFace)
	drawLegend(dc, left, 98, scienceCol, "Science (MLB win probability)")
	drawLegend(dc, left+400, 98, starsCol, "The stars (cosmic correction)")

	// Gridlines at 0/25/50/75/100%, 50% emphasized.
	for _, wp := range []float64{0, 25, 50, 75, 100} {
		alpha := 0.08
		if wp == 50 {
			alpha = 0.25
		}
		dc.SetRGBA(1, 1, 1, alpha)
		dc.SetLineWidth(1)
		dc.DrawLine(left, y(wp), right, y(wp))
		dc.Stroke()
		dc.SetColor(textCol)
		dc.DrawStringAnchored(fmt.Sprintf("%.0f%%", wp), left-14, y(wp), 1, 0.35)
	}

	// Inning ticks where each new inning starts.
	last := 0
	for i, p := range points {
		if p.Inning == last || !p.IsTop {
			continue
		}
		last = p.Inning
		dc.SetRGBA(1, 1, 1, 0.06)
		dc.DrawLine(x(i), top, x(i), bottom)
		dc.Stroke()
		dc.SetColor(textCol)
		dc.DrawStringAnchored(fmt.Sprint(p.Inning), x(i), bottom+28, 0.5, 0)
	}

	plotLine(dc, points, x, y, func(p WPPoint) float64 { return p.Real }, scienceCol, 3)
	plotLine(dc, points, x, y, func(p WPPoint) float64 { return p.Cosmic }, starsCol, 4)

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, dc.Image()); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), chartWidth, chartHeight, nil
}

func plotLine(dc *gg.Context, points []WPPoint, x func(int) float64, y func(float64) float64, val func(WPPoint) float64, col color.RGBA, width float64) {
	// A faint wide pass under the line gives it a glow.
	for _, pass := range []struct{ width, alpha float64 }{{width * 3, 0.15}, {width, 1}} {
		dc.SetRGBA(float64(col.R)/255, float64(col.G)/255, float64(col.B)/255, pass.alpha)
		dc.SetLineWidth(pass.width)
		for i, p := range points {
			if i == 0 {
				dc.MoveTo(x(i), y(val(p)))
			} else {
				dc.LineTo(x(i), y(val(p)))
			}
		}
		dc.Stroke()
	}
}

func drawLegend(dc *gg.Context, x, y float64, col color.RGBA, label string) {
	dc.SetColor(col)
	dc.SetLineWidth(4)
	dc.DrawLine(x, y, x+30, y)
	dc.Stroke()
	dc.SetColor(textCol)
	dc.DrawStringAnchored(label, x+40, y, 0, 0.35)
}

func drawChartStars(dc *gg.Context, seed string) {
	rng := newSeededRNG(seed)
	for i := 0; i < 60; i++ {
		x := rng.float64() * chartWidth
		y := rng.float64() * chartHeight
		dc.SetRGBA(1, 1, 1, 0.05+rng.float64()*0.12)
		dc.DrawCircle(x, y, 0.5+rng.float64()*1.3)
		dc.Fill()
	}
}
