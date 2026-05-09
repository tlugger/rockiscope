package web

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed dashboard.html
var dashboardHTML embed.FS

type Server struct {
	dataDir string
	logger  *log.Logger
}

func NewServer(dataDir string, logger *log.Logger) *Server {
	return &Server{dataDir: dataDir, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/predictions", s.handlePredictions)
	return mux
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := dashboardHTML.ReadFile("dashboard.html")
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
