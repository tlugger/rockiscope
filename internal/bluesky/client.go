package bluesky

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tlugger/rockiscope/internal/retry"
)

// PostRef identifies a published post for threading.
type PostRef struct {
	URI string
	CID string
}

// GetRecordCID fetches the CID for a post URI.
func (c *Client) GetRecordCID(uri string) (string, error) {
	if err := c.Authenticate(); err != nil {
		return "", err
	}

	ts := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if len(ts) < 3 {
		return "", fmt.Errorf("invalid URI: %s", uri)
	}
	repo := ts[0]
	collection := ts[1]
	rkey := ts[2]

	url := c.baseURL + "/xrpc/com.atproto.repo.getRecord?repo=" + repo + "&collection=" + collection + "&rkey=" + rkey
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessJwt)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching record: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getRecord returned status %d", resp.StatusCode)
	}

	var result struct {
		CID string `json:"cid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return result.CID, nil
}

// ExtractCID returns the CID from a Bluesky post URI.
// URI format: at://did:plc:xxx/app.bsky.feed.post/CID
func ExtractCID(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// ImageData holds a PNG image to attach to a post.
type ImageData struct {
	Bytes  []byte
	Alt    string
	Width  int
	Height int
}

// BlobRef is the Bluesky blob reference returned after upload.
type BlobRef struct {
	Type     string `json:"$type"`
	Ref      refObj `json:"ref"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
}

type refObj struct {
	Link string `json:"$link"`
}

// Poster is the interface for posting to social media.
type Poster interface {
	Post(text string, image *ImageData) (*PostRef, error)
	Reply(text string, image *ImageData, parentURI, rootURI string) (*PostRef, error)
}

// Client posts to Bluesky via the AT Protocol XRPC API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
	accessJwt  string
	did        string
	logger     *log.Logger
	sleep      func(time.Duration)
}

func NewClient(username, password string, logger *log.Logger) *Client {
	if logger == nil {
		logger = log.Default()
	}
	return &Client{
		baseURL:    "https://bsky.social",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		username:   username,
		password:   password,
		logger:     logger,
		sleep:      time.Sleep,
	}
}

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

func (c *Client) sleepFn() func(time.Duration) {
	if c.sleep != nil {
		return c.sleep
	}
	return time.Sleep
}

func (c *Client) Post(text string, image *ImageData) (*PostRef, error) {
	return retry.DoWith(c.logger, "Bluesky post", c.sleepFn(), func() (*PostRef, error) {
		if err := c.Authenticate(); err != nil {
			return nil, err
		}
		return c.createRecord(c.baseURL+"/xrpc/com.atproto.repo.createRecord", text, image, nil, nil, nil, nil)
	})
}

func (c *Client) Reply(text string, image *ImageData, parentURI, rootURI string) (*PostRef, error) {
	return retry.DoWith(c.logger, "Bluesky reply", c.sleepFn(), func() (*PostRef, error) {
		if err := c.Authenticate(); err != nil {
			return nil, err
		}
		parentCID, err := c.GetRecordCID(parentURI)
		if err != nil {
			return nil, fmt.Errorf("fetching parent CID: %w", err)
		}
		rootCID := parentCID
		if rootURI != "" && rootURI != parentURI {
			rootCID, err = c.GetRecordCID(rootURI)
			if err != nil {
				return nil, fmt.Errorf("fetching root CID: %w", err)
			}
		}
		return c.createRecord(c.baseURL+"/xrpc/com.atproto.repo.createRecord", text, image, &parentURI, &rootURI, &parentCID, &rootCID)
	})
}

func (c *Client) createRecord(url, text string, image *ImageData, parentURI, rootURI, parentCID, rootCID *string) (*PostRef, error) {
	if c.accessJwt == "" || c.did == "" {
		return nil, fmt.Errorf("not authenticated — call Authenticate() first")
	}

	rec := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	if image != nil && len(image.Bytes) > 0 {
		blob, err := c.uploadBlob(image.Bytes, "image/png")
		if err != nil {
			return nil, fmt.Errorf("uploading image: %w", err)
		}
		rec["embed"] = map[string]interface{}{
			"$type": "app.bsky.embed.images",
			"images": []map[string]interface{}{
				{
					"alt":   image.Alt,
					"image": blob,
					"aspectRatio": map[string]interface{}{
						"width":  image.Width,
						"height": image.Height,
					},
				},
			},
		}
	}

	if parentURI != nil && *parentURI != "" {
		rec["reply"] = map[string]interface{}{
			"parent": map[string]interface{}{
				"uri":  *parentURI,
				"cid":  *parentCID,
			},
			"root": map[string]interface{}{
				"uri": func() string {
					if rootURI != nil && *rootURI != "" {
						return *rootURI
					}
					return *parentURI
				}(),
				"cid": func() string {
					if rootCID != nil && *rootCID != "" {
						return *rootCID
					}
					return *parentCID
				}(),
			},
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
		var errBody struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("Bluesky post returned status %d: %s: %s", resp.StatusCode, errBody.Error, errBody.Message)
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

func (c *Client) uploadBlob(data []byte, mimeType string) (*BlobRef, error) {
	url := c.baseURL + "/xrpc/com.atproto.repo.uploadBlob"

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessJwt)
	req.Header.Set("Content-Type", mimeType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uploading blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("blob upload returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Blob *BlobRef `json:"blob"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding blob response: %w", err)
	}

	return result.Blob, nil
}

// AuthorFeedItem represents a single post from the author feed.
type AuthorFeedItem struct {
	PostURI   string
	CreatedAt string // "2006-01-02"
	Text      string
	IsReply   bool
}

// GetAuthorFeed fetches all posts from a given actor's feed using the public API.
// No authentication required.
func GetAuthorFeed(actor string) ([]AuthorFeedItem, error) {
	base := "https://public.api.bsky.app/xrpc/app.bsky.feed.getAuthorFeed"
	var items []AuthorFeedItem
	cursor := ""

	for {
		url := base + "?actor=" + actor + "&limit=100"
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("fetching author feed: %w", err)
		}

		var body struct {
			Feed []struct {
				Post struct {
					URI    string `json:"uri"`
					Record struct {
						CreatedAt string `json:"createdAt"`
						Text      string `json:"text"`
						Reply     *struct {
							Root struct {
								URI string `json:"uri"`
							} `json:"root"`
						} `json:"reply,omitempty"`
					} `json:"record"`
				} `json:"post"`
			} `json:"feed"`
			Cursor string `json:"cursor,omitempty"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding feed response: %w", err)
		}
		resp.Body.Close()

		for _, f := range body.Feed {
			created := f.Post.Record.CreatedAt
			if len(created) >= 10 {
				created = created[:10]
			}
			items = append(items, AuthorFeedItem{
				PostURI:   f.Post.URI,
				CreatedAt: created,
				Text:      f.Post.Record.Text,
				IsReply:   f.Post.Record.Reply != nil,
			})
		}

		if body.Cursor == "" {
			break
		}
		cursor = body.Cursor
	}

	return items, nil
}

// DryRunPoster implements Poster but prints to a callback instead of posting.
type DryRunPoster struct {
	OnPost  func(text string)
	OnImage func(imgBytes []byte)
	seq     int
}

func (d *DryRunPoster) Post(text string, image *ImageData) (*PostRef, error) {
	d.seq++
	if d.OnPost != nil {
		d.OnPost(text)
	}
	if d.OnImage != nil && image != nil {
		d.OnImage(image.Bytes)
	}
	return &PostRef{
		URI: fmt.Sprintf("at://dry-run/app.bsky.feed.post/%d", d.seq),
		CID: fmt.Sprintf("dry-run-cid-%d", d.seq),
	}, nil
}

func (d *DryRunPoster) Reply(text string, image *ImageData, parentURI, rootURI string) (*PostRef, error) {
	d.seq++
	if d.OnPost != nil {
		d.OnPost(text)
	}
	if d.OnImage != nil && image != nil {
		d.OnImage(image.Bytes)
	}
	return &PostRef{
		URI: fmt.Sprintf("at://dry-run/app.bsky.feed.post/%d", d.seq),
		CID: fmt.Sprintf("dry-run-cid-%d", d.seq),
	}, nil
}
