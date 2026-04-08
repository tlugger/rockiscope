package horoscope

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseHoroscopeHTML(t *testing.T) {
	html, err := os.ReadFile("../../testdata/horoscope.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	text, err := ParseHoroscopeHTML(string(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(text, "Cancer") {
		t.Errorf("expected horoscope to mention Cancer, got: %s", text)
	}
	if !strings.Contains(text, "retreat into your bedroom") {
		t.Errorf("expected specific horoscope text, got: %s", text)
	}
	if len(text) < 20 {
		t.Errorf("horoscope too short: %d chars", len(text))
	}
}

func TestParseHoroscopeHTML_NoMatch(t *testing.T) {
	_, err := ParseHoroscopeHTML("<html><body>No horoscope here</body></html>")
	if err == nil {
		t.Error("expected error for HTML without horoscope")
	}
}

func TestParseHoroscopeHTML_TooShort(t *testing.T) {
	html := `<p><strong>Apr 8, 2026</strong> - Hi.</p>`
	_, err := ParseHoroscopeHTML(html)
	if err == nil {
		t.Error("expected error for too-short horoscope")
	}
}

func TestScraper_GetDailyHoroscope(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/horoscope.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(fixture)
	}))
	defer ts.Close()

	s := &Scraper{
		baseURL:    ts.URL,
		httpClient: ts.Client(),
		sign:       "cancer",
		signID:     4,
	}

	h, err := s.GetDailyHoroscope()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.Sign != "cancer" {
		t.Errorf("sign = %q, want cancer", h.Sign)
	}
	if !strings.Contains(h.Text, "Cancer") {
		t.Errorf("expected Cancer in text, got: %s", h.Text)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 100, "short"},
		{"this is a longer sentence that should be truncated", 30, "this is a longer sentence..."},
		{"no-spaces-here-so-full-cut", 15, "no-spaces-he..."},
	}

	for _, tt := range tests {
		got := Truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
		if len(got) > tt.maxLen {
			t.Errorf("Truncate result too long: %d > %d", len(got), tt.maxLen)
		}
	}
}

func TestDecodeEntities(t *testing.T) {
	input := "Rock &amp; Roll &quot;test&quot; it&#39;s"
	want := `Rock & Roll "test" it's`
	got := decodeEntities(input)
	if got != want {
		t.Errorf("decodeEntities = %q, want %q", got, want)
	}
}
