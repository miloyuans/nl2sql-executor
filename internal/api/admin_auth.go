package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type adminSession struct {
	Username  string
	Role      string
	ExpiresAt time.Time
}

type adminUser struct {
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name,omitempty"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	Source       string    `json:"source"`
	PasswordHash string    `json:"-"`
	Salt         string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type adminUserPayload struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminUserStore struct {
	path  string
	mu    sync.Mutex
	users map[string]adminUser
}

func newAdminUserStore(path string) *adminUserStore {
	return &adminUserStore{path: path, users: map[string]adminUser{}}
}

func (s *adminUserStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var users []adminUser
	if err := json.Unmarshal(b, &users); err != nil {
		return err
	}
	s.users = map[string]adminUser{}
	for _, u := range users {
		u.Username = strings.ToLower(strings.TrimSpace(u.Username))
		if u.Username != "" {
			s.users[u.Username] = u
		}
	}
	return nil
}

func (s *adminUserStore) saveLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	users := make([]adminUser, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	b, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0600)
}

func (s *adminUserStore) ensureBootstrap() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return
	}
	now := time.Now()
	u := adminUser{Username: "admin", DisplayName: "Admin", Role: "admin", Status: "active", Source: "local", CreatedAt: now, UpdatedAt: now}
	setUserPassword(&u, firstNonBlank(os.Getenv("OPENCLAW_ADMIN_INITIAL_PASSWORD"), "admin"))
	s.users[u.Username] = u
	_ = s.saveLocked()
}

func (s *adminUserStore) list() []adminUser {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := make([]adminUser, 0, len(s.users))
	for _, u := range s.users {
		u.PasswordHash = ""
		u.Salt = ""
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	return users
}

func (s *adminUserStore) upsert(p adminUserPayload) (adminUser, error) {
	username := strings.ToLower(strings.TrimSpace(p.Username))
	if username == "" {
		return adminUser{}, fmt.Errorf("username is required")
	}
	role := strings.ToLower(strings.TrimSpace(p.Role))
	if role == "" {
		role = "user"
	}
	status := strings.ToLower(strings.TrimSpace(p.Status))
	if status == "" {
		status = "active"
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	u, exists := s.users[username]
	if !exists {
		u = adminUser{Username: username, Source: "local", CreatedAt: now}
	}
	u.DisplayName = strings.TrimSpace(p.DisplayName)
	u.Role = role
	u.Status = status
	u.UpdatedAt = now
	if strings.TrimSpace(p.Password) != "" {
		setUserPassword(&u, p.Password)
	} else if !exists {
		return adminUser{}, fmt.Errorf("password is required for new user")
	}
	s.users[username] = u
	if err := s.saveLocked(); err != nil {
		return adminUser{}, err
	}
	u.PasswordHash = ""
	u.Salt = ""
	return u, nil
}

func (s *adminUserStore) delete(username string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return fmt.Errorf("username is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, username)
	return s.saveLocked()
}

func (s *adminUserStore) authenticate(username, password string) (adminUser, bool) {
	username = strings.ToLower(strings.TrimSpace(username))
	s.mu.Lock()
	u, ok := s.users[username]
	s.mu.Unlock()
	if !ok || strings.ToLower(u.Status) != "active" {
		return adminUser{}, false
	}
	if verifyUserPassword(u, password) {
		u.PasswordHash = ""
		u.Salt = ""
		return u, true
	}
	return adminUser{}, false
}

func setUserPassword(u *adminUser, password string) {
	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	u.Salt = hex.EncodeToString(saltBytes)
	sum := sha256.Sum256([]byte(u.Salt + ":" + password))
	u.PasswordHash = hex.EncodeToString(sum[:])
}

func verifyUserPassword(u adminUser, password string) bool {
	if u.PasswordHash == "" || u.Salt == "" {
		return false
	}
	sum := sha256.Sum256([]byte(u.Salt + ":" + password))
	got := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(u.PasswordHash)) == 1
}

func (s *Server) currentAdminUser(r *http.Request) (adminUser, bool) {
	if !s.cfg.Admin.Auth.Enabled {
		return adminUser{Username: "noauth", DisplayName: "No Auth", Role: "admin", Status: "active", Source: "system"}, true
	}
	if tokenEnv := strings.TrimSpace(s.cfg.Admin.Auth.AdminTokenEnv); tokenEnv != "" {
		want := strings.TrimSpace(os.Getenv(tokenEnv))
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(strings.TrimSpace(got))) == 1 {
			return adminUser{Username: "api-token", Role: "admin", Status: "active", Source: "token"}, true
		}
	}
	cookie, err := r.Cookie(s.cfg.Admin.Auth.CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return adminUser{}, false
	}
	s.sessionsMu.Lock()
	sess, ok := s.sessions[cookie.Value]
	if ok && time.Now().After(sess.ExpiresAt) {
		delete(s.sessions, cookie.Value)
		ok = false
	}
	s.sessionsMu.Unlock()
	if !ok {
		return adminUser{}, false
	}
	return adminUser{Username: sess.Username, Role: sess.Role, Status: "active", Source: "session"}, true
}

func (s *Server) requireAdminAPI(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := s.currentAdminUser(r); ok {
		return true
	}
	writeJSON(w, http.StatusUnauthorized, errResp("admin login required"))
	return false
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, u adminUser) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	ttl := time.Duration(s.cfg.Admin.Auth.SessionTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	exp := time.Now().Add(ttl)
	s.sessionsMu.Lock()
	s.sessions[token] = adminSession{Username: u.Username, Role: u.Role, ExpiresAt: exp}
	s.sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.cfg.Admin.Auth.CookieName, Value: token, Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil})
}

func (s *Server) clearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.cfg.Admin.Auth.CookieName); err == nil {
		s.sessionsMu.Lock()
		delete(s.sessions, c.Value)
		s.sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: s.cfg.Admin.Auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
