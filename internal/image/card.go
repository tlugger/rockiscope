package image

import (
	"bytes"
	"image/color"
	"image/png"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	cardWidth = 800
	padding   = 40
	textWidth = cardWidth - (padding * 2)
)

var (
	bgTop     = color.RGBA{R: 25, G: 15, B: 50, A: 255}
	bgBottom  = color.RGBA{R: 45, G: 20, B: 80, A: 255}
	textCol   = color.RGBA{R: 230, G: 220, B: 255, A: 255}
	accentCol = color.RGBA{R: 200, G: 170, B: 255, A: 255}
)

// HoroscopeCard generates a styled PNG image of the horoscope text.
func HoroscopeCard(horoscopeText string) ([]byte, int, int, error) {
	fontRegular, err := truetype.Parse(goregular.TTF)
	if err != nil {
		return nil, 0, 0, err
	}
	fontBold, err := truetype.Parse(gobold.TTF)
	if err != nil {
		return nil, 0, 0, err
	}

	bodyFace := truetype.NewFace(fontRegular, &truetype.Options{Size: 18})
	headerFace := truetype.NewFace(fontBold, &truetype.Options{Size: 20})

	// Measure body text to calculate card height
	measure := gg.NewContext(cardWidth, 1)
	measure.SetFontFace(bodyFace)
	lines := measure.WordWrap(horoscopeText, float64(textWidth))
	lineHeight := 18 * 1.6
	bodyHeight := float64(len(lines)) * lineHeight

	headerBlock := 20.0 + 12.0 + 1.0 + 16.0
	cardHeight := int(float64(padding) + headerBlock + bodyHeight + float64(padding))
	if cardHeight < 150 {
		cardHeight = 150
	}

	dc := gg.NewContext(cardWidth, cardHeight)

	// Gradient background
	for y := 0; y < cardHeight; y++ {
		t := float64(y) / float64(cardHeight)
		r := lerp(float64(bgTop.R), float64(bgBottom.R), t) / 255
		g := lerp(float64(bgTop.G), float64(bgBottom.G), t) / 255
		b := lerp(float64(bgTop.B), float64(bgBottom.B), t) / 255
		dc.SetRGB(r, g, b)
		dc.DrawRectangle(0, float64(y), float64(cardWidth), 1)
		dc.Fill()
	}

	// Background decorations
	drawCrescentMoon(dc, 700, 55, 22)
	drawConstellation(dc, cardHeight)
	drawScatteredStars(dc, cardHeight, horoscopeText)

	y := float64(padding)

	// Header
	dc.SetFontFace(headerFace)
	dc.SetColor(accentCol)
	dc.DrawString("Cancer  --  "+time.Now().Format("January 2, 2006"), padding, y+20)
	y += 20 + 12

	// Divider
	dc.SetRGBA(1, 1, 1, 0.15)
	dc.DrawLine(padding, y, float64(cardWidth)-padding, y)
	dc.Stroke()
	y += 16

	// Body text
	dc.SetFontFace(bodyFace)
	dc.SetColor(textCol)
	dc.DrawStringWrapped(horoscopeText, padding, y, 0, 0, float64(textWidth), 1.6, gg.AlignLeft)

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, dc.Image()); err != nil {
		return nil, 0, 0, err
	}

	return buf.Bytes(), cardWidth, cardHeight, nil
}

// drawCrescentMoon draws a crescent moon using two overlapping circles.
func drawCrescentMoon(dc *gg.Context, cx, cy, radius float64) {
	// Soft glow around the moon
	for i := 4; i >= 0; i-- {
		r := radius + float64(i)*7
		alpha := 0.06 - float64(i)*0.01
		if alpha > 0 {
			dc.SetRGBA(0.8, 0.7, 1, alpha)
			dc.DrawCircle(cx, cy, r)
			dc.Fill()
		}
	}

	// Outer circle (the lit side)
	dc.SetRGBA(1, 1, 1, 0.20)
	dc.DrawCircle(cx, cy, radius)
	dc.Fill()

	// Inner circle offset to carve the crescent
	dc.SetRGBA(0.10, 0.06, 0.22, 1) // approximate bg color at that height
	dc.DrawCircle(cx+radius*0.45, cy-radius*0.15, radius*0.85)
	dc.Fill()
}

// drawConstellation draws the Cancer constellation pattern (connected stars).
// Placed in the upper-right area, behind the text.
func drawConstellation(dc *gg.Context, cardHeight int) {
	// Cancer constellation: an inverted-Y shape with a few branches.
	// Offset to upper-right corner so it doesn't fight the text.
	ox, oy := 620.0, 100.0

	// Star positions (relative to origin)
	stars := [][2]float64{
		{0, 0},      // Acubens
		{30, -35},   // Al Tarf
		{65, -20},   // Asellus Borealis
		{60, 15},    // Asellus Australis
		{95, -5},    // Praesepe cluster center
		{120, -30},  // Delta Cancri
		{85, 30},    // Iota Cancri
	}

	// Connections between star indices
	connections := [][2]int{
		{0, 1}, {1, 2}, {1, 3}, {2, 4}, {3, 4}, {4, 5}, {3, 6},
	}

	// Adjust if constellation would go off bottom
	if oy+40 > float64(cardHeight) {
		oy = float64(cardHeight) - 60
	}

	// Draw connection lines
	dc.SetRGBA(1, 1, 1, 0.06)
	dc.SetLineWidth(1)
	for _, conn := range connections {
		a := stars[conn[0]]
		b := stars[conn[1]]
		dc.DrawLine(ox+a[0], oy+a[1], ox+b[0], oy+b[1])
		dc.Stroke()
	}

	// Draw stars as dots with subtle glow
	for _, s := range stars {
		x, y := ox+s[0], oy+s[1]
		// Glow
		dc.SetRGBA(0.8, 0.7, 1, 0.05)
		dc.DrawCircle(x, y, 4)
		dc.Fill()
		// Star dot
		dc.SetRGBA(1, 1, 1, 0.15)
		dc.DrawCircle(x, y, 1.8)
		dc.Fill()
	}
}

// drawScatteredStars places stars randomly, seeded from the horoscope text
// so the pattern is unique each day but deterministic for the same text.
func drawScatteredStars(dc *gg.Context, cardHeight int, seed string) {
	rng := newSeededRNG(seed)
	count := 25 + rng.intn(15) // 25-40 stars

	for i := 0; i < count; i++ {
		x := rng.float64() * float64(cardWidth)
		y := rng.float64() * float64(cardHeight)
		size := 0.5 + rng.float64()*1.2
		brightness := 0.10 + rng.float64()*0.15

		// Soft halo
		dc.SetRGBA(1, 1, 1, brightness*0.3)
		dc.DrawCircle(x, y, size*3)
		dc.Fill()
		// Bright core
		dc.SetRGBA(1, 1, 1, brightness)
		dc.DrawCircle(x, y, size)
		dc.Fill()
	}
}

// Simple seeded RNG to avoid importing math/rand for a few numbers.
// Uses a basic xorshift from a hash of the seed string.
type seededRNG struct {
	state uint64
}

func newSeededRNG(seed string) *seededRNG {
	var h uint64 = 5381
	for _, c := range seed {
		h = h*33 + uint64(c)
	}
	if h == 0 {
		h = 1
	}
	return &seededRNG{state: h}
}

func (r *seededRNG) next() uint64 {
	r.state ^= r.state << 13
	r.state ^= r.state >> 7
	r.state ^= r.state << 17
	return r.state
}

func (r *seededRNG) float64() float64 {
	return float64(r.next()%10000) / 10000.0
}

func (r *seededRNG) intn(n int) int {
	return int(r.next() % uint64(n))
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}
