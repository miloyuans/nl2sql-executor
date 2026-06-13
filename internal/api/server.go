package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/datasource"
	"nl2sql-executor-go-prod/internal/job"
)

type Server struct {
	cfg        *config.Config
	mgr        *job.Manager
	ds         *datasource.Manager
	mux        *http.ServeMux
	users      *adminUserStore
	sessions   map[string]adminSession
	sessionsMu sync.Mutex
}

func NewServer(cfg *config.Config, mgr *job.Manager, ds *datasource.Manager) *Server {
	store := newAdminUserStore(cfg.Admin.Users.File)
	_ = store.load()
	if cfg.Admin.Auth.Enabled {
		store.ensureBootstrap()
	}
	s := &Server{cfg: cfg, mgr: mgr, ds: ds, mux: http.NewServeMux(), users: store, sessions: map[string]adminSession{}}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.health)
	s.mux.HandleFunc("/readyz", s.ready)
	s.mux.HandleFunc("/", s.root)
	s.mux.HandleFunc("/admin", s.adminIndex)
	s.mux.HandleFunc("/admin/login", s.adminLoginPage)
	s.mux.HandleFunc("/admin/sso/login", s.adminSSOLogin)
	s.mux.HandleFunc("/admin/sso/callback", s.adminSSOCallback)
	s.mux.HandleFunc("/v1/query-jobs", s.queryJobs)
	s.mux.HandleFunc("/v1/jobs/", s.getJob)
	s.mux.HandleFunc("/v1/admin/jobs", s.adminJobs)
	s.mux.HandleFunc("/v1/admin/jobs/", s.adminJobAction)
	s.mux.HandleFunc("/v1/admin/sql/execute", s.adminSQLExecute)
	s.mux.HandleFunc("/v1/admin/users", s.adminUsers)
	s.mux.HandleFunc("/v1/admin/users/", s.adminUserAction)
	s.mux.HandleFunc("/v1/admin/settings", s.adminSettings)
	s.mux.HandleFunc("/v1/admin/auth/login", s.adminAuthLogin)
	s.mux.HandleFunc("/v1/admin/auth/logout", s.adminAuthLogout)
	s.mux.HandleFunc("/v1/admin/auth/me", s.adminAuthMe)
	s.mux.HandleFunc("/v1/admin/schema/export", s.adminSchemaExport)
	s.mux.HandleFunc("/v1/admin/schema/exports", s.adminSchemaExports)
	s.mux.HandleFunc("/v1/admin/schema/download", s.adminSchemaDownload)
	s.mux.HandleFunc("/v1/datasources", s.datasources)
}

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	http.NotFound(w, r)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func errResp(msg string) map[string]any { return map[string]any{"error": msg} }
