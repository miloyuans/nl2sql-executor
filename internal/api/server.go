package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/datasource"
	"nl2sql-executor-go-prod/internal/job"
)

type Server struct {
	cfg *config.Config
	mgr *job.Manager
	ds  *datasource.Manager
	mux *http.ServeMux
}

func NewServer(cfg *config.Config, mgr *job.Manager, ds *datasource.Manager) *Server {
	s := &Server{cfg: cfg, mgr: mgr, ds: ds, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler { return s.authMiddleware(s.mux) }

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.health)
	s.mux.HandleFunc("/readyz", s.ready)
	s.mux.HandleFunc("/v1/query-jobs", s.queryJobs)
	s.mux.HandleFunc("/v1/jobs/", s.getJob)
	s.mux.HandleFunc("/v1/datasources", s.datasources)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "datasources": s.ds.Ping(ctx)})
}

func (s *Server) datasources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"datasources": s.ds.Status()})
}

func (s *Server) queryJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	defer r.Body.Close()
	var req job.QueryRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid json: "+err.Error()))
		return
	}
	if strings.TrimSpace(req.ChatID) == "" || strings.TrimSpace(req.SQL) == "" {
		writeJSON(w, http.StatusBadRequest, errResp("chat_id and sql are required"))
		return
	}
	j, err := s.mgr.Submit(req)
	if err != nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"job_id": j.ID, "status": j.Status, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": j.ID, "status": j.Status})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errResp("missing job id"))
		return
	}
	j, ok := s.mgr.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errResp("job not found"))
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("X-API-Key")
		if key == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				key = strings.TrimSpace(auth[7:])
			}
		}
		if key == "" || subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.Auth.APIKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, errResp("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func errResp(msg string) map[string]any { return map[string]any{"error": msg} }
