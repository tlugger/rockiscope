package bluesky

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mockBlueskyServer(t *testing.T, onPost func(body map[string]interface{})) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "createSession") {
			json.NewEncoder(w).Encode(map[string]string{
				"accessJwt": "fake-jwt-token",
				"did":       "did:plc:fake123",
			})
			return
		}

		if strings.Contains(r.URL.Path, "uploadBlob") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"blob": map[string]interface{}{
					"$type":    "blob",
					"ref":      map[string]string{"$link": "bafkreifakeblob"},
					"mimeType": "image/png",
					"size":     1234,
				},
			})
			return
		}

		if strings.Contains(r.URL.Path, "createRecord") {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if onPost != nil {
				onPost(body)
			}
			json.NewEncoder(w).Encode(map[string]string{
				"uri": "at://did:plc:fake123/app.bsky.feed.post/abc123",
				"cid": "bafyreifake123",
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestAuthenticate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["identifier"] != "test.bsky.social" {
			t.Errorf("unexpected identifier: %s", body["identifier"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"accessJwt": "fake-jwt-token",
			"did":       "did:plc:fake123",
		})
	}))
	defer ts.Close()

	c := &Client{
		baseURL: ts.URL, httpClient: ts.Client(),
		username: "test.bsky.social", password: "test-password",
	}

	err := c.authenticateWithURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.accessJwt != "fake-jwt-token" {
		t.Errorf("jwt = %q", c.accessJwt)
	}
}

func TestAuthenticate_BadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	c := &Client{httpClient: ts.Client(), username: "bad", password: "creds"}
	err := c.authenticateWithURL(ts.URL)
	if err == nil {
		t.Error("expected error for 401")
	}
}

func TestPost_TextOnly(t *testing.T) {
	var receivedText string
	ts := mockBlueskyServer(t, func(body map[string]interface{}) {
		record := body["record"].(map[string]interface{})
		receivedText = record["text"].(string)
		if _, ok := record["embed"]; ok {
			t.Error("text-only post should not have embed")
		}
	})
	defer ts.Close()

	c := &Client{baseURL: ts.URL, httpClient: ts.Client(), username: "test", password: "test"}
	ref, err := c.Post("Hello!", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedText != "Hello!" {
		t.Errorf("text = %q", receivedText)
	}
	if ref.URI == "" || ref.CID == "" {
		t.Error("expected non-empty PostRef")
	}
}

func TestPost_WithImage(t *testing.T) {
	var hasEmbed bool
	ts := mockBlueskyServer(t, func(body map[string]interface{}) {
		record := body["record"].(map[string]interface{})
		if embed, ok := record["embed"]; ok {
			hasEmbed = true
			embedMap := embed.(map[string]interface{})
			if embedMap["$type"] != "app.bsky.embed.images" {
				t.Errorf("embed type = %q", embedMap["$type"])
			}
		}
	})
	defer ts.Close()

	c := &Client{baseURL: ts.URL, httpClient: ts.Client(), username: "test", password: "test"}
	img := &ImageData{Bytes: []byte("fake-png"), Alt: "test image", Width: 800, Height: 420}
	_, err := c.Post("Post with image", img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasEmbed {
		t.Error("expected embed in post with image")
	}
}

func TestPost_NotAuthenticated(t *testing.T) {
	c := &Client{baseURL: "http://localhost:1", httpClient: http.DefaultClient, username: "x", password: "x"}
	_, err := c.Post("test", nil)
	if err == nil {
		t.Error("expected error when auth server unreachable")
	}
}

func TestDryRunPoster(t *testing.T) {
	var posts []string
	var gotImage bool
	drp := &DryRunPoster{
		OnPost:  func(text string) { posts = append(posts, text) },
		OnImage: func(b []byte) { gotImage = true },
	}

	ref, err := drp.Post("main post", &ImageData{Bytes: []byte("png")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.URI == "" {
		t.Error("expected non-empty URI")
	}
	if !gotImage {
		t.Error("expected OnImage callback")
	}
	if len(posts) != 1 {
		t.Errorf("expected 1 post, got %d", len(posts))
	}
}

func TestReply_ReplyRecordStructure(t *testing.T) {
	var rec map[string]interface{}
	ts := mockBlueskyServer(t, func(body map[string]interface{}) {
		rec = body["record"].(map[string]interface{})
	})
	defer ts.Close()

	c := &Client{baseURL: ts.URL, httpClient: ts.Client(), username: "test", password: "test"}
	parentURI := "at://did:plc:abc123/app.bsky.feed.post/parent123"
	rootURI := "at://did:plc:abc123/app.bsky.feed.post/root456"

	_, err := c.Reply("reply text", nil, parentURI, rootURI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reply, ok := rec["reply"].(map[string]interface{})
	if !ok {
		t.Fatal("expected reply field in record")
	}

	parent, ok := reply["parent"].(map[string]interface{})
	if !ok {
		t.Fatal("expected parent as object")
	}
	if parent["uri"] != parentURI {
		t.Errorf("parent.uri = %q, want %q", parent["uri"], parentURI)
	}
	if _, ok := parent["cid"]; !ok {
		t.Error("expected parent.cid")
	}

	root, ok := reply["root"].(map[string]interface{})
	if !ok {
		t.Fatal("expected root as object, got string")
	}
	if root["uri"] != rootURI {
		t.Errorf("root.uri = %q, want %q", root["uri"], rootURI)
	}
	if _, ok := root["cid"]; !ok {
		t.Error("expected root.cid")
	}
}

func TestReply_UsesParentAsRoot(t *testing.T) {
	var rec map[string]interface{}
	ts := mockBlueskyServer(t, func(body map[string]interface{}) {
		rec = body["record"].(map[string]interface{})
	})
	defer ts.Close()

	c := &Client{baseURL: ts.URL, httpClient: ts.Client(), username: "test", password: "test"}
	parentURI := "at://did:plc:abc123/app.bsky.feed.post/parent123"

	_, err := c.Reply("reply text", nil, parentURI, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reply := rec["reply"].(map[string]interface{})
	root := reply["root"].(map[string]interface{})
	if root["uri"] != parentURI {
		t.Errorf("root.uri = %q, want %q (defaults to parentURI)", root["uri"], parentURI)
	}
}
