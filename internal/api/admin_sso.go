package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

func (s *Server) adminSSOLogin(w http.ResponseWriter, r *http.Request) {
	if !s.ssoEnabled() {
		writeJSON(w, http.StatusBadRequest, errResp("SSO 未启用或配置不完整"))
		return
	}
	d, err := s.oidcDiscover(r)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errResp(err.Error()))
		return
	}
	state := randomURLToken(24)
	http.SetCookie(w, &http.Cookie{Name: "openclaw_oidc_state", Value: state, Path: "/admin", MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil})
	q := url.Values{}
	q.Set("client_id", s.cfg.Admin.SSO.ClientID)
	q.Set("redirect_uri", s.cfg.Admin.SSO.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", firstNonBlank(s.cfg.Admin.SSO.Scopes, "openid profile email"))
	q.Set("state", state)
	http.Redirect(w, r, d.AuthorizationEndpoint+"?"+q.Encode(), http.StatusFound)
}

func (s *Server) adminSSOCallback(w http.ResponseWriter, r *http.Request) {
	if !s.ssoEnabled() {
		writeJSON(w, http.StatusBadRequest, errResp("SSO 未启用或配置不完整"))
		return
	}
	cookie, err := r.Cookie("openclaw_oidc_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		writeJSON(w, http.StatusBadRequest, errResp("invalid sso state"))
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeJSON(w, http.StatusBadRequest, errResp("missing sso code"))
		return
	}
	d, err := s.oidcDiscover(r)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errResp(err.Error()))
		return
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.cfg.Admin.SSO.RedirectURL)
	form.Set("client_id", s.cfg.Admin.SSO.ClientID)
	if strings.TrimSpace(s.cfg.Admin.SSO.ClientSecret) != "" {
		form.Set("client_secret", s.cfg.Admin.SSO.ClientSecret)
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errResp(err.Error()))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, errResp("sso token exchange failed: "+string(body)))
		return
	}
	var tok oidcTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		writeJSON(w, http.StatusBadGateway, errResp("invalid token response: "+err.Error()))
		return
	}
	if tok.Error != "" {
		writeJSON(w, http.StatusBadGateway, errResp(tok.Error+": "+tok.Description))
		return
	}
	claims := map[string]any{}
	if d.UserInfoEndpoint != "" && tok.AccessToken != "" {
		ureq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, d.UserInfoEndpoint, nil)
		ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		uresp, err := http.DefaultClient.Do(ureq)
		if err == nil {
			defer uresp.Body.Close()
			ubody, _ := io.ReadAll(io.LimitReader(uresp.Body, 4<<20))
			_ = json.Unmarshal(ubody, &claims)
		}
	}
	username := claimString(claims, "preferred_username")
	if username == "" {
		username = claimString(claims, "email")
	}
	if username == "" {
		username = claimString(claims, "sub")
	}
	if username == "" {
		writeJSON(w, http.StatusForbidden, errResp("SSO 登录成功，但未获取到用户名"))
		return
	}
	role := "user"
	if s.isSSOAdmin(username, claims) {
		role = "admin"
	}
	u := adminUser{Username: strings.ToLower(username), DisplayName: firstNonBlank(claimString(claims, "name"), username), Role: role, Status: "active", Source: "sso", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.createSession(w, r, u)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) ssoEnabled() bool {
	return s.cfg.Admin.SSO.Enabled && strings.TrimSpace(s.cfg.Admin.SSO.IssuerURL) != "" && strings.TrimSpace(s.cfg.Admin.SSO.ClientID) != "" && strings.TrimSpace(s.cfg.Admin.SSO.RedirectURL) != ""
}

func (s *Server) oidcDiscover(r *http.Request) (oidcDiscovery, error) {
	issuer := strings.TrimRight(s.cfg.Admin.SSO.IssuerURL, "/")
	resp, err := http.Get(issuer + "/.well-known/openid-configuration")
	if err != nil {
		return oidcDiscovery{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return oidcDiscovery{}, fmt.Errorf("oidc discovery failed: %s", resp.Status)
	}
	var d oidcDiscovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&d); err != nil {
		return oidcDiscovery{}, err
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" {
		return oidcDiscovery{}, fmt.Errorf("oidc discovery missing authorization/token endpoints")
	}
	return d, nil
}

func (s *Server) isSSOAdmin(username string, claims map[string]any) bool {
	username = strings.ToLower(strings.TrimSpace(username))
	for _, u := range s.cfg.Admin.SSO.AdminUsers {
		if strings.ToLower(strings.TrimSpace(u)) == username {
			return true
		}
	}
	roles := map[string]struct{}{}
	collectClaimRoles(roles, claims["roles"])
	collectClaimRoles(roles, claims["groups"])
	if realm, ok := claims["realm_access"].(map[string]any); ok {
		collectClaimRoles(roles, realm["roles"])
	}
	for _, r := range s.cfg.Admin.SSO.AdminRoles {
		if _, ok := roles[strings.ToLower(strings.TrimSpace(r))]; ok {
			return true
		}
	}
	return false
}

func collectClaimRoles(out map[string]struct{}, v any) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
			}
		}
	case []string:
		for _, s := range x {
			out[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
		}
	case string:
		for _, p := range strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
			if strings.TrimSpace(p) != "" {
				out[strings.ToLower(strings.TrimSpace(p))] = struct{}{}
			}
		}
	}
}

func claimString(claims map[string]any, key string) string {
	if v, ok := claims[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func randomURLToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
