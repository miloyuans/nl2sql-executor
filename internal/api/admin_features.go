package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/datasource"
)

type adminSettingsPayload struct {
	Auth            config.AdminAuthConfig   `json:"auth"`
	SSO             adminSSOPublic           `json:"sso"`
	Telegram        adminTelegramSettings    `json:"telegram"`
	Queue           adminQueueSettings       `json:"queue"`
	SchemaExport    adminSchemaExportSetting `json:"schema_export"`
	DatasourceCount int                      `json:"datasource_count"`
}

type adminSSOPublic struct {
	Enabled         bool     `json:"enabled"`
	IssuerURL       string   `json:"issuer_url"`
	ClientID        string   `json:"client_id"`
	ClientSecretSet bool     `json:"client_secret_set"`
	RedirectURL     string   `json:"redirect_url"`
	Scopes          string   `json:"scopes"`
	AdminUsers      []string `json:"admin_users"`
	AdminRoles      []string `json:"admin_roles"`
	UserRoles       []string `json:"user_roles"`
}

type adminSettingsUpdate struct {
	SSO              *config.AdminSSOConfig   `json:"sso"`
	Telegram         *adminTelegramSettings   `json:"telegram"`
	SchemaExport     *adminSchemaExportConfig `json:"schema_export"`
	KeepClientSecret bool                     `json:"keep_client_secret"`
}

type adminTelegramSettings struct {
	CompactResultOnly bool `json:"compact_result_only"`
	SendCSV           bool `json:"send_csv"`
	SendChartSVG      bool `json:"send_chart_svg"`
	MaxInlineRows     int  `json:"max_inline_rows"`
}

type adminQueueSettings struct {
	Workers           int  `json:"workers"`
	BufferSize        int  `json:"buffer_size"`
	MaxPerUserRunning int  `json:"max_per_user_running"`
	NotifyOnAccepted  bool `json:"notify_on_accepted"`
	JobTimeoutSec     int  `json:"job_timeout_sec"`
}

type adminSchemaExportSetting struct {
	Dir                  string   `json:"dir"`
	MaxRows              int      `json:"max_rows"`
	IncludeSystemSchemas bool     `json:"include_system_schemas"`
	SystemSchemas        []string `json:"system_schemas"`
}

type adminSchemaExportConfig = adminSchemaExportSetting

type schemaExportRequest struct {
	DatasourceID         string `json:"data_source_id"`
	IncludeSystemSchemas bool   `json:"include_system_schemas"`
	SendToChatID         string `json:"send_to_chat_id"`
}

func (s *Server) adminAuthMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentAdminUser(r)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": ok, "auth_enabled": s.cfg.Admin.Auth.Enabled, "user": u, "sso_enabled": s.cfg.Admin.SSO.Enabled})
}

func (s *Server) adminAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	defer r.Body.Close()
	var req adminLoginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid json: "+err.Error()))
		return
	}
	u, ok := s.users.authenticate(req.Username, req.Password)
	if !ok {
		log.Printf("admin login failed username=%s remote=%s", req.Username, r.RemoteAddr)
		writeJSON(w, http.StatusUnauthorized, errResp("用户名或密码错误"))
		return
	}
	log.Printf("admin login success username=%s remote=%s", u.Username, r.RemoteAddr)
	s.createSession(w, r, u)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u})
}

func (s *Server) adminAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Admin.Auth.Enabled {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminLoginHTML(s.cfg.Admin.SSO.Enabled)))
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"users": s.users.list()})
	case http.MethodPost:
		defer r.Body.Close()
		var req adminUserPayload
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp("invalid json: "+err.Error()))
			return
		}
		u, err := s.users.upsert(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": u})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
	}
}

func (s *Server) adminUserAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	username := strings.TrimPrefix(r.URL.Path, "/v1/admin/users/")
	username = strings.Trim(username, "/")
	if username == "" {
		writeJSON(w, http.StatusBadRequest, errResp("username is required"))
		return
	}
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	if err := s.users.delete(username); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.publicSettings())
	case http.MethodPost:
		defer r.Body.Close()
		var req adminSettingsUpdate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp("invalid json: "+err.Error()))
			return
		}
		if req.SSO != nil {
			oldSecret := s.cfg.Admin.SSO.ClientSecret
			next := *req.SSO
			if req.KeepClientSecret && strings.TrimSpace(next.ClientSecret) == "" {
				next.ClientSecret = oldSecret
			}
			if strings.TrimSpace(next.Scopes) == "" {
				next.Scopes = "openid profile email"
			}
			s.cfg.Admin.SSO = next
		}
		if req.Telegram != nil {
			compact := req.Telegram.CompactResultOnly
			s.cfg.Telegram.CompactResultOnly = &compact
			s.cfg.Telegram.SendCSV = req.Telegram.SendCSV
			s.cfg.Telegram.SendChartSVG = req.Telegram.SendChartSVG
			if req.Telegram.MaxInlineRows > 0 {
				s.cfg.Telegram.MaxInlineRows = req.Telegram.MaxInlineRows
			}
		}
		if req.SchemaExport != nil {
			s.cfg.Admin.SchemaExport.Dir = strings.TrimSpace(req.SchemaExport.Dir)
			s.cfg.Admin.SchemaExport.MaxRows = req.SchemaExport.MaxRows
			s.cfg.Admin.SchemaExport.IncludeSystemSchemas = req.SchemaExport.IncludeSystemSchemas
			s.cfg.Admin.SchemaExport.SystemSchemas = cleanList(req.SchemaExport.SystemSchemas)
		}
		writeJSON(w, http.StatusOK, s.publicSettings())
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
	}
}

func (s *Server) publicSettings() adminSettingsPayload {
	return adminSettingsPayload{
		Auth:            s.cfg.Admin.Auth,
		SSO:             adminSSOPublic{Enabled: s.cfg.Admin.SSO.Enabled, IssuerURL: s.cfg.Admin.SSO.IssuerURL, ClientID: s.cfg.Admin.SSO.ClientID, ClientSecretSet: strings.TrimSpace(s.cfg.Admin.SSO.ClientSecret) != "", RedirectURL: s.cfg.Admin.SSO.RedirectURL, Scopes: s.cfg.Admin.SSO.Scopes, AdminUsers: s.cfg.Admin.SSO.AdminUsers, AdminRoles: s.cfg.Admin.SSO.AdminRoles, UserRoles: s.cfg.Admin.SSO.UserRoles},
		Telegram:        adminTelegramSettings{CompactResultOnly: s.cfg.Telegram.IsCompactResultOnly(), SendCSV: s.cfg.Telegram.SendCSV, SendChartSVG: s.cfg.Telegram.SendChartSVG, MaxInlineRows: s.cfg.Telegram.MaxInlineRows},
		Queue:           adminQueueSettings{Workers: s.cfg.Queue.Workers, BufferSize: s.cfg.Queue.BufferSize, MaxPerUserRunning: s.cfg.Queue.MaxPerUserRunning, NotifyOnAccepted: s.cfg.Queue.NotifyOnAccepted, JobTimeoutSec: s.cfg.Queue.JobTimeoutSec},
		SchemaExport:    adminSchemaExportSetting{Dir: s.cfg.Admin.SchemaExport.Dir, MaxRows: s.cfg.Admin.SchemaExport.MaxRows, IncludeSystemSchemas: s.cfg.Admin.SchemaExport.IncludeSystemSchemas, SystemSchemas: s.cfg.Admin.SchemaExport.SystemSchemas},
		DatasourceCount: len(s.cfg.Datasources.Items),
	}
}

func (s *Server) adminSchemaExport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	defer r.Body.Close()
	var req schemaExportRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&req)
	dsID := strings.TrimSpace(req.DatasourceID)
	if dsID == "" {
		dsID = s.ds.DefaultID()
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(maxInt(s.cfg.Queue.JobTimeoutSec, 300))*time.Second)
	defer cancel()
	log.Printf("admin schema export start datasource=%s include_system_schemas=%t", dsID, req.IncludeSystemSchemas || s.cfg.Admin.SchemaExport.IncludeSystemSchemas)
	exp, err := s.ds.ExportSchema(ctx, dsID, datasource.SchemaExportOptions{IncludeSystemSchemas: req.IncludeSystemSchemas || s.cfg.Admin.SchemaExport.IncludeSystemSchemas, SystemSchemas: s.cfg.Admin.SchemaExport.SystemSchemas, MaxRows: s.cfg.Admin.SchemaExport.MaxRows})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errResp(err.Error()))
		return
	}
	jsonPath, mdPath, err := writeSchemaExportFiles(s.cfg.Admin.SchemaExport.Dir, exp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	log.Printf("admin schema export complete datasource=%s json=%s markdown=%s db=%d table=%d view=%d column=%d index=%d warnings=%d", dsID, filepath.Base(jsonPath), filepath.Base(mdPath), exp.Summary.DatabaseCount, exp.Summary.TableCount, exp.Summary.ViewCount, exp.Summary.ColumnCount, exp.Summary.IndexCount, len(exp.Errors))
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":               exp.Summary,
		"errors":                exp.Errors,
		"json_file":             filepath.Base(jsonPath),
		"markdown_file":         filepath.Base(mdPath),
		"json_download_url":     "/v1/admin/schema/download?file=" + filepath.Base(jsonPath),
		"markdown_download_url": "/v1/admin/schema/download?file=" + filepath.Base(mdPath),
	})
}

func (s *Server) adminSchemaExports(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	files, _ := filepath.Glob(filepath.Join(s.cfg.Admin.SchemaExport.Dir, "*"))
	items := make([]map[string]any, 0, len(files))
	for _, p := range files {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		items = append(items, map[string]any{"file": filepath.Base(p), "size": info.Size(), "updated_at": info.ModTime(), "download_url": "/v1/admin/schema/download?file=" + filepath.Base(p)})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["updated_at"].(time.Time).After(items[j]["updated_at"].(time.Time))
	})
	writeJSON(w, http.StatusOK, map[string]any{"exports": items})
}

func (s *Server) adminSchemaDownload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	name := filepath.Base(r.URL.Query().Get("file"))
	if name == "." || name == string(filepath.Separator) || strings.TrimSpace(name) == "" {
		writeJSON(w, http.StatusBadRequest, errResp("file is required"))
		return
	}
	path := filepath.Join(s.cfg.Admin.SchemaExport.Dir, name)
	if _, err := os.Stat(path); err != nil {
		writeJSON(w, http.StatusNotFound, errResp("file not found"))
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasSuffix(strings.ToLower(name), ".json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	} else if strings.HasSuffix(strings.ToLower(name), ".md") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	log.Printf("admin schema download file=%s", name)
	http.ServeFile(w, r, path)
}

func writeSchemaExportFiles(dir string, exp *datasource.SchemaExport) (string, string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", err
	}
	stamp := exp.GeneratedAt.Format("20060102_150405")
	base := fmt.Sprintf("schema_%s_%s", sanitizeFilePart(exp.DatasourceID), stamp)
	jsonPath := filepath.Join(dir, base+".json")
	mdPath := filepath.Join(dir, base+".md")
	b, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(jsonPath, b, 0644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(mdPath, []byte(schemaExportMarkdown(exp)), 0644); err != nil {
		return "", "", err
	}
	return jsonPath, mdPath, nil
}

func schemaExportMarkdown(exp *datasource.SchemaExport) string {
	var b strings.Builder
	b.WriteString("# OpenClaw 数据库架构导出\n\n")
	b.WriteString(fmt.Sprintf("- 数据源：`%s`\n- 节点：`%s`\n- 导出时间：`%s`\n- 库：%d，表：%d，视图：%d，字段：%d，索引：%d\n\n", exp.DatasourceID, exp.Host, exp.GeneratedAt.Format(time.RFC3339), exp.Summary.DatabaseCount, exp.Summary.TableCount, exp.Summary.ViewCount, exp.Summary.ColumnCount, exp.Summary.IndexCount))
	if len(exp.Errors) > 0 {
		b.WriteString("## 导出警告\n\n")
		for _, e := range exp.Errors {
			b.WriteString("- " + e + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## 数据库\n\n| schema | charset | collation |\n|---|---|---|\n")
	for _, d := range exp.Databases {
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", mdEscape(d.SchemaName), mdEscape(d.DefaultCharset), mdEscape(d.DefaultCollation)))
	}
	b.WriteString("\n## 表与视图\n\n| schema | object | type | engine | rows | comment |\n|---|---|---|---|---:|---|\n")
	for _, t := range exp.Tables {
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s | %s | %s |\n", mdEscape(t.SchemaName), mdEscape(t.TableName), mdEscape(t.TableType), mdEscape(t.Engine), mdEscape(t.TableRows), mdEscape(t.TableComment)))
	}
	b.WriteString("\n## 字段\n\n| schema | table | column | type | nullable | key | comment |\n|---|---|---|---|---|---|---|\n")
	for _, c := range exp.Columns {
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s | %s | %s | %s |\n", mdEscape(c.SchemaName), mdEscape(c.TableName), mdEscape(c.ColumnName), mdEscape(firstNonBlank(c.ColumnType, c.DataType)), mdEscape(c.Nullable), mdEscape(c.ColumnKey), mdEscape(c.Comment)))
	}
	b.WriteString("\n## 索引\n\n| schema | table | index | unique | seq | column | type | comment |\n|---|---|---|---|---:|---|---|---|\n")
	for _, idx := range exp.Indexes {
		unique := "YES"
		if idx.NonUnique == "1" {
			unique = "NO"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s | %s | `%s` | %s | %s |\n", mdEscape(idx.SchemaName), mdEscape(idx.TableName), mdEscape(idx.IndexName), unique, mdEscape(idx.SeqInIndex), mdEscape(idx.ColumnName), mdEscape(idx.IndexType), mdEscape(idx.Comment)))
	}
	b.WriteString("\n## 视图定义\n\n")
	for _, v := range exp.Views {
		b.WriteString(fmt.Sprintf("### `%s`.`%s`\n\n```sql\n%s\n```\n\n", mdEscape(v.SchemaName), mdEscape(v.ViewName), v.ViewDefinition))
	}
	return b.String()
}

func sanitizeFilePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
