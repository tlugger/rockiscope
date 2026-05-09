package web

import (
	"encoding/json"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "[test] ", 0)
}

func TestHandleDashboard(t *testing.T) {
	srv := NewServer("", testLogger())
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "Rockiscope Analytics") {
		t.Error("expected dashboard title in response")
	}
}

func TestHandleDashboard_NotFound(t *testing.T) {
	srv := NewServer("", testLogger())
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandlePredictions_NoFile(t *testing.T) {
	dir := t.TempDir()
	srv := NewServer(dir, testLogger())
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/predictions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json, got %s", ct)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
}

func TestHandlePredictions_WithFile(t *testing.T) {
	dir := t.TempDir()
	history := `{"predictions":[{"date":"2026-04-08","predicted":"W","actual":"W"}],"currentWeights":{"winRate":0.25}}`
	if err := os.WriteFile(filepath.Join(dir, "prediction_history.json"), []byte(history), 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(dir, testLogger())
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/predictions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	preds, ok := data["predictions"].([]interface{})
	if !ok || len(preds) != 1 {
		t.Errorf("expected 1 prediction, got %v", data["predictions"])
	}
}

func TestHandlePredictions_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prediction_history.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(dir, testLogger())
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/predictions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500 for invalid json, got %d", w.Code)
	}
}

func TestNewServer(t *testing.T) {
	srv := NewServer("/tmp", testLogger())
	if srv == nil {
		t.Fatal("expected server")
	}
	handler := srv.Handler()
	if handler == nil {
		t.Fatal("expected handler")
	}
}

func TestHandler_Routes(t *testing.T) {
	srv := NewServer(t.TempDir(), testLogger())
	handler := srv.Handler()

	tests := []struct {
		path string
		code int
	}{
		{"/", 200},
		{"/api/predictions", 200},
		{"/favicon.ico", 404},
		{"/api/other", 404},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tt.code {
			t.Errorf("%s: expected %d, got %d", tt.path, tt.code, w.Code)
		}
	}
}
