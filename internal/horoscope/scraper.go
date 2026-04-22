package horoscope

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Horoscope represents a daily horoscope reading.
type Horoscope struct {
	Sign string
	Text string
}

// Provider is the interface for fetching horoscopes.
type Provider interface {
	GetDailyHoroscope() (*Horoscope, error)
}

// Scraper fetches daily horoscopes from horoscope.com.
type Scraper struct {
	baseURL    string
	httpClient *http.Client
	sign       string
	signID     int
}

var signIDs = map[string]int{
	"aries": 1, "taurus": 2, "gemini": 3, "cancer": 4,
	"leo": 5, "virgo": 6, "libra": 7, "scorpio": 8,
	"sagittarius": 9, "capricorn": 10, "aquarius": 11, "pisces": 12,
}

func NewScraper(httpClient *http.Client) *Scraper {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Scraper{
		baseURL:    "https://www.horoscope.com",
		httpClient: httpClient,
		sign:       "cancer",
		signID:     4,
	}
}

func (s *Scraper) GetDailyHoroscope() (*Horoscope, error) {
	url := fmt.Sprintf("%s/us/horoscopes/general/horoscope-general-daily-today.aspx?sign=%d", s.baseURL, s.signID)
	return s.fetchFromURL(url)
}

func (s *Scraper) fetchFromURL(url string) (*Horoscope, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Rockiscope/2.0)")
	req.Header.Set("Accept", "text/html")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching horoscope: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("horoscope.com returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	text, err := ParseHoroscopeHTML(string(body))
	if err != nil {
		return nil, err
	}

	return &Horoscope{Sign: s.sign, Text: text}, nil
}

// ParseHoroscopeHTML extracts the horoscope text from horoscope.com HTML.
// Exported so tests can call it directly with fixture HTML.
func ParseHoroscopeHTML(html string) (string, error) {
	// Pattern: <p><strong>DATE</strong> - HOROSCOPE TEXT</p>
	re := regexp.MustCompile(`<p><strong>[^<]+</strong>\s*-\s*(.*?)</p>`)
	matches := re.FindStringSubmatch(html)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not find horoscope text in HTML")
	}

	text := matches[1]
	text = stripTags(text)
	text = decodeEntities(text)
	text = normalizeWhitespace(text)
	text = strings.ReplaceAll(text, "Cancer", "Rockies")

	if len(text) < 20 {
		return "", fmt.Errorf("horoscope text too short (%d chars)", len(text))
	}

	return text, nil
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}

func decodeEntities(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&quot;", "\"",
		"&apos;", "'",
		"&lt;", "<",
		"&gt;", ">",
		"&nbsp;", " ",
		"&#39;", "'",
	)
	return r.Replace(s)
}

func normalizeWhitespace(s string) string {
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(s, " "))
}

// Truncate shortens text to maxLen, ending at a word boundary with "...".
func Truncate(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	truncated := text[:maxLen-3]
	if idx := strings.LastIndex(truncated, " "); idx > 0 {
		truncated = truncated[:idx]
	}
	return truncated + "..."
}
