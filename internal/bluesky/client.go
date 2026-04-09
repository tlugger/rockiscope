package bluesky

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PostRef identifies a published post for threading.
type PostRef struct {
	URI string
	CID string
}

// Poster is the interface for posting to social media.
type Poster interface {
	Post(text string) (*PostRef, error)
	Reply(text string, parent PostRef) (*PostRef, error)
}

// Client posts to Bluesky via the AT Protocol XRPC API.
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

func (c *Client) Post(text string) (*PostRef, error) {
	return c.createRecord(c.baseURL+"/xrpc/com.atproto.repo.createRecord", text, nil)
}

func (c *Client) Reply(text string, parent PostRef) (*PostRef, error) {
	return c.createRecord(c.baseURL+"/xrpc/com.atproto.repo.createRecord", text, &parent)
}

func (c *Client) createRecord(url, text string, parent *PostRef) (*PostRef, error) {
	if c.accessJwt == "" || c.did == "" {
		return nil, fmt.Errorf("not authenticated — call Authenticate() first")
	}

	rec := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	if parent != nil {
		ref := map[string]interface{}{
			"uri": parent.URI,
			"cid": parent.CID,
		}
		rec["reply"] = map[string]interface{}{
			"root":   ref,
			"parent": ref,
		}
	}

	payload := map[string]interface{}{
		"repo":       c.did,
		"collection": "app.bsky.feed.post",
		"record":     rec,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessJwt)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("posting to Bluesky: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bluesky post returned status %d", resp.StatusCode)
	}

	var result struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding post response: %w", err)
	}

	return &PostRef{URI: result.URI, CID: result.CID}, nil
}

// DryRunPoster implements Poster but just prints to a callback instead of posting.
type DryRunPoster struct {
	OnPost func(text string)
	seq    int
}

func (d *DryRunPoster) Post(text string) (*PostRef, error) {
	d.seq++
	if d.OnPost != nil {
		d.OnPost(text)
	}
	return &PostRef{
		URI: fmt.Sprintf("at://dry-run/app.bsky.feed.post/%d", d.seq),
		CID: fmt.Sprintf("dry-run-cid-%d", d.seq),
	}, nil
}

func (d *DryRunPoster) Reply(text string, parent PostRef) (*PostRef, error) {
	d.seq++
	if d.OnPost != nil {
		d.OnPost(text)
	}
	return &PostRef{
		URI: fmt.Sprintf("at://dry-run/app.bsky.feed.post/%d", d.seq),
		CID: fmt.Sprintf("dry-run-cid-%d", d.seq),
	}, nil
}
