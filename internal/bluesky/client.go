package bluesky

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Poster is the interface for posting to social media.
type Poster interface {
	Post(text string) error
}

// Client posts to Bluesky via the AT Protocol XRPC API.
// Uses direct HTTP calls for simplicity — no heavy SDK dependency.
type Client struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
	accessJwt  string
	did        string
}

func NewClient(username, password string) *Client {
	return &Client{
		baseURL:    "https://bsky.social",
		httpClient: &http.Client{Timeout: 15 * time.Second},
		username:   username,
		password:   password,
	}
}

// Authenticate creates a session with Bluesky. Must be called before Post.
func (c *Client) Authenticate() error {
	return c.authenticateWithURL(c.baseURL + "/xrpc/com.atproto.server.createSession")
}

func (c *Client) authenticateWithURL(url string) error {
	body, _ := json.Marshal(map[string]string{
		"identifier": c.username,
		"password":   c.password,
	})

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("authenticating with Bluesky: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bluesky auth returned status %d", resp.StatusCode)
	}

	var result struct {
		AccessJwt string `json:"accessJwt"`
		DID       string `json:"did"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding auth response: %w", err)
	}

	if result.AccessJwt == "" || result.DID == "" {
		return fmt.Errorf("empty auth credentials in response")
	}

	c.accessJwt = result.AccessJwt
	c.did = result.DID
	return nil
}

// Post publishes a text post to Bluesky.
func (c *Client) Post(text string) error {
	return c.postWithURL(c.baseURL+"/xrpc/com.atproto.repo.createRecord", text)
}

func (c *Client) postWithURL(url, text string) error {
	if c.accessJwt == "" || c.did == "" {
		return fmt.Errorf("not authenticated — call Authenticate() first")
	}

	record := map[string]interface{}{
		"repo":       c.did,
		"collection": "app.bsky.feed.post",
		"record": map[string]interface{}{
			"$type":     "app.bsky.feed.post",
			"text":      text,
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		},
	}

	body, _ := json.Marshal(record)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessJwt)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting to Bluesky: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bluesky post returned status %d", resp.StatusCode)
	}

	return nil
}

// DryRunPoster implements Poster but just prints to a callback instead of posting.
type DryRunPoster struct {
	OnPost func(text string)
}

func (d *DryRunPoster) Post(text string) error {
	if d.OnPost != nil {
		d.OnPost(text)
	}
	return nil
}
