package bluesky

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake-jwt" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		record := body["record"].(map[string]interface{})
		receivedText = record["text"].(string)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"uri":"at://did:plc:fake/app.bsky.feed.post/abc123"}`))
	}))
	defer ts.Close()

	c := &Client{
		httpClient: ts.Client(),
		accessJwt:  "fake-jwt",
		did:        "did:plc:fake",
	}

	err := c.postWithURL(ts.URL, "Hello from Rockiscope!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedText != "Hello from Rockiscope!" {
		t.Errorf("post text = %q, want %q", receivedText, "Hello from Rockiscope!")
	}
}

func TestPost_NotAuthenticated(t *testing.T) {
	c := &Client{}
	err := c.postWithURL("http://localhost", "test")
	if err == nil {
		t.Error("expected error when not authenticated")
	}
}

func TestDryRunPoster(t *testing.T) {
	var captured string
	drp := &DryRunPoster{
		OnPost: func(text string) { captured = text },
	}

	err := drp.Post("test post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured != "test post" {
		t.Errorf("captured = %q, want %q", captured, "test post")
	}
}
