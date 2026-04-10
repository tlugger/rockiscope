package bluesky

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockBlueskyServer handles both auth and post endpoints.
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
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

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
		baseURL:    ts.URL,
		httpClient: ts.Client(),
		username:   "test.bsky.social",
		password:   "test-password",
	}

	err := c.authenticateWithURL(ts.URL + "/xrpc/com.atproto.server.createSession")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.accessJwt != "fake-jwt-token" {
		t.Errorf("jwt = %q, want fake-jwt-token", c.accessJwt)
	}
	if c.did != "did:plc:fake123" {
		t.Errorf("did = %q, want did:plc:fake123", c.did)
	}
}

func TestAuthenticate_BadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		username:   "bad", password: "creds",
	}

	err := c.authenticateWithURL(ts.URL)
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestPost(t *testing.T) {
	var receivedText string

	ts := mockBlueskyServer(t, func(body map[string]interface{}) {
		record := body["record"].(map[string]interface{})
		receivedText = record["text"].(string)

		if _, ok := record["reply"]; ok {
			t.Error("root post should not have reply field")
		}
	})
	defer ts.Close()

	c := &Client{
		baseURL:    ts.URL,
		httpClient: ts.Client(),
		username:   "test.bsky.social",
		password:   "test-password",
	}

	ref, err := c.Post("Hello from Rockiscope!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedText != "Hello from Rockiscope!" {
		t.Errorf("post text = %q, want %q", receivedText, "Hello from Rockiscope!")
	}

	if ref.URI == "" || ref.CID == "" {
		t.Errorf("expected non-empty PostRef, got URI=%q CID=%q", ref.URI, ref.CID)
	}
}

func TestReply(t *testing.T) {
	var receivedReply map[string]interface{}

	ts := mockBlueskyServer(t, func(body map[string]interface{}) {
		record := body["record"].(map[string]interface{})
		receivedReply, _ = record["reply"].(map[string]interface{})
	})
	defer ts.Close()

	c := &Client{
		baseURL:    ts.URL,
		httpClient: ts.Client(),
		username:   "test.bsky.social",
		password:   "test-password",
	}

	parent := PostRef{
		URI: "at://did:plc:fake/app.bsky.feed.post/abc123",
		CID: "bafyreifake123",
	}

	ref, err := c.Reply("Horoscope thread reply", parent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedReply == nil {
		t.Fatal("expected reply field in record")
	}

	root := receivedReply["root"].(map[string]interface{})
	if root["uri"] != parent.URI {
		t.Errorf("root URI = %q, want %q", root["uri"], parent.URI)
	}
	if root["cid"] != parent.CID {
		t.Errorf("root CID = %q, want %q", root["cid"], parent.CID)
	}

	if ref.URI == "" {
		t.Error("expected non-empty reply URI")
	}
}

func TestPost_NotAuthenticated(t *testing.T) {
	// With ensureAuth, a client with no baseURL/httpClient will fail on auth
	c := &Client{
		baseURL:    "http://localhost:1",
		httpClient: http.DefaultClient,
		username:   "test",
		password:   "test",
	}
	_, err := c.Post("test")
	if err == nil {
		t.Error("expected error when auth server unreachable")
	}
}

func TestDryRunPoster(t *testing.T) {
	var posts []string
	drp := &DryRunPoster{
		OnPost: func(text string) { posts = append(posts, text) },
	}

	ref1, err := drp.Post("main post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref1.URI == "" || ref1.CID == "" {
		t.Error("expected non-empty PostRef from dry run")
	}

	ref2, err := drp.Reply("reply post", *ref1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref2.URI == ref1.URI {
		t.Error("reply should have different URI than parent")
	}

	if len(posts) != 2 {
		t.Errorf("expected 2 posts captured, got %d", len(posts))
	}
}
