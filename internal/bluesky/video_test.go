package bluesky

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// videoWorld fakes the PDS, the video service and MLB's CDN on one server.
type videoWorld struct {
	t           *testing.T
	canUpload   bool
	uploadFails bool
	jobPolls    int

	mu       sync.Mutex
	embed    map[string]interface{}
	paths    []string
	auds     []string
	uploaded int
}

func (w *videoWorld) serve(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paths = append(w.paths, r.URL.Path)
	rw.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(rw)
	switch {
	case strings.HasSuffix(r.URL.Path, "createSession"):
		enc.Encode(map[string]interface{}{
			"accessJwt": "jwt", "did": "did:plc:bot",
			"didDoc": map[string]interface{}{"service": []map[string]string{
				{"id": "#atproto_pds", "serviceEndpoint": "https://morel.us-east.host.bsky.network"},
			}},
		})
	case strings.HasSuffix(r.URL.Path, "getServiceAuth"):
		w.auds = append(w.auds, r.URL.Query().Get("aud")+" "+r.URL.Query().Get("lxm"))
		enc.Encode(map[string]string{"token": "svc"})
	case strings.HasSuffix(r.URL.Path, "getUploadLimits"):
		enc.Encode(map[string]interface{}{"canUpload": w.canUpload, "remainingDailyVideos": 20, "remainingDailyBytes": 1 << 30, "message": "quota"})
	case strings.HasSuffix(r.URL.Path, "uploadVideo"):
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer svc" || r.URL.Query().Get("did") != "did:plc:bot" {
			w.t.Errorf("upload auth/did wrong: %s %s", r.Header.Get("Authorization"), r.URL.RawQuery)
		}
		w.uploaded = len(body)
		if w.uploadFails {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		enc.Encode(map[string]interface{}{"jobId": "job1", "state": "JOB_STATE_CREATED"})
	case strings.HasSuffix(r.URL.Path, "getJobStatus"):
		w.jobPolls++
		st := map[string]interface{}{"jobId": "job1", "state": "JOB_STATE_ENCODING"}
		if w.jobPolls >= 2 {
			st = map[string]interface{}{"jobId": "job1", "state": "JOB_STATE_COMPLETED",
				"blob": map[string]interface{}{"$type": "blob", "ref": map[string]string{"$link": "bafyvideo"}, "mimeType": "video/mp4", "size": 7}}
		}
		enc.Encode(map[string]interface{}{"jobStatus": st})
	case strings.HasSuffix(r.URL.Path, "uploadBlob"):
		enc.Encode(map[string]interface{}{"blob": map[string]interface{}{"$type": "blob", "ref": map[string]string{"$link": "bafythumb"}, "mimeType": "image/jpeg", "size": 5}})
	case strings.HasSuffix(r.URL.Path, "getRecord"):
		enc.Encode(map[string]string{"cid": "cid-" + r.URL.Query().Get("rkey")})
	case strings.HasSuffix(r.URL.Path, "createRecord"):
		var body struct {
			Record map[string]interface{} `json:"record"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.embed, _ = body.Record["embed"].(map[string]interface{})
		enc.Encode(map[string]string{"uri": "at://did:plc:bot/app.bsky.feed.post/new", "cid": "c"})
	case strings.HasSuffix(r.URL.Path, ".mp4"):
		rw.Write([]byte("fakemp4"))
	case strings.HasSuffix(r.URL.Path, ".jpg"):
		rw.Write([]byte("thumb"))
	default:
		http.NotFound(rw, r)
	}
}

func newVideoWorld(t *testing.T, w *videoWorld) (*Client, string) {
	w.t = t
	srv := httptest.NewServer(http.HandlerFunc(w.serve))
	t.Cleanup(srv.Close)
	c := NewClient("bot", "pw", testLogger())
	c.baseURL = srv.URL
	c.videoURL = srv.URL
	c.mediaClient = srv.Client()
	c.sleep = func(time.Duration) {}
	return c, srv.URL
}

func testClip(base string) *Clip {
	return &Clip{
		VideoURL: base + "/cuts/asset_1280x720_59_4000K.mp4",
		PageURL:  "https://www.mlb.com/video/aranda-single",
		Title:    "Jonathan Aranda's RBI single",
		ThumbURL: base + "/thumb.jpg",
		Alt:      "Jonathan Aranda singles to left",
	}
}

const parent = "at://did:plc:bot/app.bsky.feed.post/parent"

func TestReplyClip_Video(t *testing.T) {
	w := &videoWorld{canUpload: true}
	c, base := newVideoWorld(t, w)
	if _, err := c.ReplyClip("homer!", testClip(base), parent, parent); err != nil {
		t.Fatal(err)
	}
	if w.embed["$type"] != "app.bsky.embed.video" {
		t.Fatalf("embed = %v", w.embed)
	}
	if w.uploaded != len("fakemp4") || w.jobPolls != 2 {
		t.Errorf("uploaded %d bytes, polled %d times", w.uploaded, w.jobPolls)
	}
	// Upload tokens are for the account's PDS, not the bsky.social entryway.
	if !strings.Contains(strings.Join(w.auds, ","), "did:web:morel.us-east.host.bsky.network com.atproto.repo.uploadBlob") {
		t.Errorf("service auth audiences = %v", w.auds)
	}
}

func TestReplyClip_FallsBackToLinkCard(t *testing.T) {
	for name, w := range map[string]*videoWorld{
		"quota exhausted": {canUpload: false},
		"upload fails":    {canUpload: true, uploadFails: true},
	} {
		t.Run(name, func(t *testing.T) {
			c, base := newVideoWorld(t, w)
			if _, err := c.ReplyClip("homer!", testClip(base), parent, parent); err != nil {
				t.Fatal(err)
			}
			if w.embed["$type"] != "app.bsky.embed.external" {
				t.Fatalf("embed = %v", w.embed)
			}
			ext := w.embed["external"].(map[string]interface{})
			if ext["uri"] != "https://www.mlb.com/video/aranda-single" || ext["thumb"] == nil {
				t.Errorf("external = %v", ext)
			}
		})
	}
}

func TestReplyClip_NilClipIsPlainReply(t *testing.T) {
	w := &videoWorld{canUpload: true}
	c, _ := newVideoWorld(t, w)
	if _, err := c.ReplyClip("no clip", nil, parent, parent); err != nil {
		t.Fatal(err)
	}
	if w.embed != nil {
		t.Errorf("embed = %v", w.embed)
	}
}
