package web

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

//go:embed dashboard.html favicon.ico
var staticFiles embed.FS

type Server struct {
	dataDir string
	logger  *log.Logger
	now     func() time.Time
}

func NewServer(dataDir string, logger *log.Logger) *Server {
	return &Server{dataDir: dataDir, logger: logger, now: time.Now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/favicon.ico", s.handleFavicon)
	mux.HandleFunc("/api/predictions", s.handlePredictions)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/seasons", s.handleSeasons)
	mux.HandleFunc("GET /api/season/{year}", s.handleSeason)
	return mux
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("dashboard.html")
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("favicon.ico")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

func (s *Server) handlePredictions(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.dataDir, "prediction_history.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"predictions":[],"currentWeights":{}}`))
		return
	}
	if err != nil {
		http.Error(w, "could not read prediction history", 500)
		return
	}

	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		http.Error(w, "invalid prediction history", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
