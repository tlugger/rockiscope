package bluesky

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/tlugger/rockiscope/internal/retry"
)

const (
	defaultVideoURL = "https://video.bsky.app"
	// maxVideoBytes bounds a clip download. MLB's 720p highlights run ~0.5MB
	// per second, so this is minutes of footage, far beyond any single play.
	maxVideoBytes = 100 << 20
	// maxThumbBytes is Bluesky's limit for a link-card thumbnail.
	maxThumbBytes = 1 << 20
	// Video processing usually takes 10-30s for a highlight.
	jobPollInterval = 2 * time.Second
	jobTimeout      = 3 * time.Minute
)

// Clip is a highlight to attach to a post: inline video when possible, a link
// card when not.
type Clip struct {
	VideoURL    string // mp4
	PageURL     string // where the link card points
	Title       string
	Description string
	ThumbURL    string
	Alt         string
}

// ClipPoster can reply with a highlight clip.
type ClipPoster interface {
	Poster
	ReplyClip(text string, clip *Clip, parentURI, rootURI string) (*PostRef, error)
}

// ReplyClip replies with clip embedded. It tries inline video first, falls
// back to a link card (out of daily video quota, upload or processing
// failure), and finally to plain text, so a flaky video service never costs
// us the post itself.
func (c *Client) ReplyClip(text string, clip *Clip, parentURI, rootURI string) (*PostRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := retry.RunWith(c.logger, "Bluesky auth", c.sleepFn(), c.Authenticate); err != nil {
		return nil, err
	}

	var embed map[string]interface{}
	if clip != nil {
		embed = c.clipEmbed(clip)
	}
	return retry.DoWith(c.logger, "Bluesky clip reply", c.sleepFn(), func() (*PostRef, error) {
		if err := c.Authenticate(); err != nil {
			return nil, err
		}
		return c.replyWithEmbed(text, embed, parentURI, rootURI)
	})
}

func (c *Client) clipEmbed(clip *Clip) map[string]interface{} {
	if clip.VideoURL != "" {
		embed, err := c.videoEmbed(clip)
		if err == nil {
			return embed
		}
		c.logger.Printf("warning: video upload failed, falling back to a link card: %v", err)
	}
	embed, err := c.linkCardEmbed(clip)
	if err != nil {
		c.logger.Printf("warning: link card failed, posting text only: %v", err)
		return nil
	}
	return embed
}

func (c *Client) videoEmbed(clip *Clip) (map[string]interface{}, error) {
	limits, err := c.uploadLimits()
	if err != nil {
		return nil, fmt.Errorf("checking upload limits: %w", err)
	}
	if !limits.CanUpload {
		return nil, fmt.Errorf("video upload not allowed: %s", limits.Message)
	}

	data, err := c.download(clip.VideoURL, maxVideoBytes)
	if err != nil {
		return nil, err
	}
	if limits.RemainingDailyBytes > 0 && int64(len(data)) > limits.RemainingDailyBytes {
		return nil, fmt.Errorf("clip is %d bytes, only %d left today", len(data), limits.RemainingDailyBytes)
	}

	blob, err := c.uploadVideo(data, path.Base(clip.VideoURL))
	if err != nil {
		return nil, err
	}
	c.logger.Printf("uploaded clip (%d bytes, %d videos left today)", len(data), limits.RemainingDailyVideos-1)
	return map[string]interface{}{
		"$type":       "app.bsky.embed.video",
		"video":       blob,
		"alt":         clip.Alt,
		"aspectRatio": map[string]int{"width": 16, "height": 9},
	}, nil
}

func (c *Client) linkCardEmbed(clip *Clip) (map[string]interface{}, error) {
	uri := clip.PageURL
	if uri == "" {
		uri = clip.VideoURL
	}
	if uri == "" {
		return nil, fmt.Errorf("clip has no URL")
	}
	external := map[string]interface{}{
		"uri":         uri,
		"title":       clip.Title,
		"description": clip.Description,
	}
	if clip.ThumbURL != "" {
		if img, err := c.download(clip.ThumbURL, maxThumbBytes); err != nil {
			c.logger.Printf("warning: skipping link-card thumbnail: %v", err)
		} else if blob, err := c.uploadBlob(img, "image/jpeg"); err != nil {
			c.logger.Printf("warning: skipping link-card thumbnail: %v", err)
		} else {
			external["thumb"] = blob
		}
	}
	return map[string]interface{}{"$type": "app.bsky.embed.external", "external": external}, nil
}

type uploadLimits struct {
	CanUpload            bool   `json:"canUpload"`
	RemainingDailyVideos int    `json:"remainingDailyVideos"`
	RemainingDailyBytes  int64  `json:"remainingDailyBytes"`
	Message              string `json:"message"`
}

func (c *Client) uploadLimits() (*uploadLimits, error) {
	token, err := c.serviceAuth("did:web:"+hostOf(c.videoBase()), "app.bsky.video.getUploadLimits")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", c.videoBase()+"/xrpc/app.bsky.video.getUploadLimits", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var limits uploadLimits
	if err := c.doJSON(req, &limits); err != nil {
		return nil, err
	}
	return &limits, nil
}

type jobStatus struct {
	JobID   string   `json:"jobId"`
	State   string   `json:"state"`
	Blob    *BlobRef `json:"blob"`
	Error   string   `json:"error"`
	Message string   `json:"message"`
}

// uploadVideo sends the clip to Bluesky's video service and waits for it to
// finish processing, returning the blob to embed.
func (c *Client) uploadVideo(data []byte, name string) (*BlobRef, error) {
	// The upload token is for our PDS: the video service writes the processed
	// blob to it on our behalf.
	token, err := c.serviceAuth("did:web:"+hostOf(c.pdsURL), "com.atproto.repo.uploadBlob")
	if err != nil {
		return nil, err
	}
	q := url.Values{"did": {c.did}, "name": {name}}
	req, err := http.NewRequest("POST", c.videoBase()+"/xrpc/app.bsky.video.uploadVideo?"+q.Encode(), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "video/mp4")
	req.ContentLength = int64(len(data))

	resp, err := c.media().Do(req)
	if err != nil {
		return nil, fmt.Errorf("uploading video: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// The service answers with a bare job status, but the lexicon wraps it in
	// {jobStatus}; accept both. A 409 means this exact file was uploaded
	// before, and its job can still be polled.
	var out struct {
		jobStatus
		Wrapped *jobStatus `json:"jobStatus"`
	}
	_ = json.Unmarshal(body, &out)
	job := out.jobStatus
	if out.Wrapped != nil {
		job = *out.Wrapped
	}
	if resp.StatusCode != http.StatusOK && !(resp.StatusCode == http.StatusConflict && job.JobID != "") {
		return nil, fmt.Errorf("video upload returned status %d: %s", resp.StatusCode, string(body))
	}
	if job.Blob != nil {
		return job.Blob, nil
	}
	if job.JobID == "" {
		return nil, fmt.Errorf("video upload returned no job: %s", string(body))
	}
	return c.awaitJob(job.JobID)
}

func (c *Client) awaitJob(jobID string) (*BlobRef, error) {
	sleep := c.sleepFn()
	for waited := time.Duration(0); waited < jobTimeout; waited += jobPollInterval {
		req, err := http.NewRequest("GET", c.videoBase()+"/xrpc/app.bsky.video.getJobStatus?jobId="+url.QueryEscape(jobID), nil)
		if err != nil {
			return nil, err
		}
		var out struct {
			JobStatus jobStatus `json:"jobStatus"`
		}
		if err := c.doJSON(req, &out); err != nil {
			c.logger.Printf("warning: checking video job %s: %v", jobID, err)
		} else {
			switch st := out.JobStatus; {
			case st.Blob != nil:
				return st.Blob, nil
			case st.State == "JOB_STATE_FAILED":
				return nil, fmt.Errorf("video processing failed: %s %s", st.Error, st.Message)
			}
		}
		sleep(jobPollInterval)
	}
	return nil, fmt.Errorf("video job %s still processing after %s", jobID, jobTimeout)
}

// serviceAuth mints a short-lived token for calling another service as us.
func (c *Client) serviceAuth(aud, lxm string) (string, error) {
	q := url.Values{
		"aud": {aud},
		"lxm": {lxm},
		"exp": {fmt.Sprint(time.Now().Add(30 * time.Minute).Unix())},
	}
	req, err := http.NewRequest("GET", c.baseURL+"/xrpc/com.atproto.server.getServiceAuth?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessJwt)
	var out struct {
		Token string `json:"token"`
	}
	if err := c.doJSON(req, &out); err != nil {
		return "", fmt.Errorf("getting service auth for %s: %w", lxm, err)
	}
	return out.Token, nil
}

func (c *Client) download(rawURL string, limit int64) ([]byte, error) {
	resp, err := c.media().Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: status %d", rawURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", rawURL, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is over %d bytes", rawURL, limit)
	}
	return data, nil
}

func (c *Client) doJSON(req *http.Request, v interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s returned status %d: %s", req.URL.Path, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) videoBase() string {
	if c.videoURL != "" {
		return c.videoURL
	}
	return defaultVideoURL
}

// media is the client for big transfers, which outlast the API timeout.
func (c *Client) media() *http.Client {
	if c.mediaClient != nil {
		return c.mediaClient
	}
	return &http.Client{Timeout: 3 * time.Minute}
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}

func (d *DryRunPoster) ReplyClip(text string, clip *Clip, parentURI, rootURI string) (*PostRef, error) {
	if clip != nil {
		text += "\n[clip: " + clip.Title + " " + clip.VideoURL + "]"
	}
	return d.Reply(text, nil, parentURI, rootURI)
}
