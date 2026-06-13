package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ListOptions struct {
	Status string
	Query  string
	Limit  int
	Offset int
}

type ListResult struct {
	Total int       `json:"total"`
	Jobs  []JobItem `json:"jobs"`
}

type JobItem struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	Question       string    `json:"question,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
	ChatID         string    `json:"chat_id,omitempty"`
	DatasourceID   string    `json:"data_source_id,omitempty"`
	SQL            string    `json:"sql,omitempty"`
	RewrittenSQL   string    `json:"rewritten_sql,omitempty"`
	ResultText     string    `json:"result_text,omitempty"`
	ResultRowCount int       `json:"result_row_count,omitempty"`
	ResultDuration int64     `json:"result_duration_ms,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (m *Manager) List(opt ListOptions) ListResult {
	m.loadPersistedJobs()
	m.mu.Lock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.Unlock()

	status := strings.ToLower(strings.TrimSpace(opt.Status))
	q := strings.ToLower(strings.TrimSpace(opt.Query))
	items := make([]JobItem, 0, len(jobs))
	for _, j := range jobs {
		if status != "" && strings.ToLower(j.Status) != status {
			continue
		}
		if q != "" && !jobMatchesQuery(j, q) {
			continue
		}
		items = append(items, toJobItem(j))
	}
	sort.Slice(items, func(i, k int) bool {
		return items[i].CreatedAt.After(items[k].CreatedAt)
	})
	total := len(items)
	if opt.Offset < 0 {
		opt.Offset = 0
	}
	if opt.Limit <= 0 || opt.Limit > 200 {
		opt.Limit = 50
	}
	if opt.Offset >= len(items) {
		return ListResult{Total: total, Jobs: []JobItem{}}
	}
	end := opt.Offset + opt.Limit
	if end > len(items) {
		end = len(items)
	}
	return ListResult{Total: total, Jobs: items[opt.Offset:end]}
}

func (m *Manager) Rerun(id string) (*Job, error) {
	old, ok := m.Get(id)
	if !ok {
		return nil, os.ErrNotExist
	}
	req := old.Request
	req.RequestID = ""
	// Re-runs should query the datasource again rather than reusing a cached result.
	req.CacheKey = ""
	if strings.TrimSpace(req.Question) == "" {
		req.Question = "重跑任务 " + old.ID
	}
	return m.Submit(req)
}

func (m *Manager) Get(id string) (*Job, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, false
	}
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if ok {
		return j, true
	}
	j, ok = m.loadJobFile(id)
	if !ok {
		return nil, false
	}
	m.mu.Lock()
	m.jobs[id] = j
	m.mu.Unlock()
	return j, true
}

func (m *Manager) loadPersistedJobs() {
	files, err := filepath.Glob(filepath.Join(m.cfg.Storage.JobDir, "*.json"))
	if err != nil {
		return
	}
	for _, path := range files {
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		m.mu.Lock()
		_, exists := m.jobs[id]
		m.mu.Unlock()
		if exists {
			continue
		}
		if j, ok := m.loadJobPath(path); ok {
			m.mu.Lock()
			m.jobs[j.ID] = j
			m.mu.Unlock()
		}
	}
}

func (m *Manager) loadJobFile(id string) (*Job, bool) {
	return m.loadJobPath(filepath.Join(m.cfg.Storage.JobDir, id+".json"))
}

func (m *Manager) loadJobPath(path string) (*Job, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var j Job
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, false
	}
	if strings.TrimSpace(j.ID) == "" {
		return nil, false
	}
	return &j, true
}

func toJobItem(j *Job) JobItem {
	sqlText := j.RewrittenSQL
	if strings.TrimSpace(sqlText) == "" {
		sqlText = j.Request.SQL
	}
	return JobItem{
		ID:             j.ID,
		Status:         j.Status,
		Error:          j.Error,
		Question:       j.Request.Question,
		UserID:         j.Request.UserID,
		ChatID:         j.Request.ChatID,
		DatasourceID:   firstNonEmpty(j.DatasourceID, j.Request.DatasourceID),
		SQL:            j.Request.SQL,
		RewrittenSQL:   sqlText,
		ResultText:     j.ResultText,
		ResultRowCount: j.ResultRowCount,
		ResultDuration: j.ResultDuration,
		CreatedAt:      j.CreatedAt,
		UpdatedAt:      j.UpdatedAt,
	}
}

func jobMatchesQuery(j *Job, q string) bool {
	fields := []string{
		j.ID,
		j.Status,
		j.Error,
		j.Request.Question,
		j.Request.UserID,
		j.Request.ChatID,
		j.Request.SQL,
		j.RewrittenSQL,
		j.ResultText,
		j.DatasourceID,
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
