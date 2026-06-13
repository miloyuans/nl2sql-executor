package job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nl2sql-executor-go-prod/internal/cache"
	"nl2sql-executor-go-prod/internal/chart"
	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/datasource"
	"nl2sql-executor-go-prod/internal/dbresult"
	"nl2sql-executor-go-prod/internal/formatter"
	"nl2sql-executor-go-prod/internal/schema"
	"nl2sql-executor-go-prod/internal/sqlguard"
	"nl2sql-executor-go-prod/internal/telegram"
)

type QueryRequest struct {
	RequestID    string     `json:"request_id"`
	UserID       string     `json:"user_id"`
	ChatID       string     `json:"chat_id"`
	SessionID    string     `json:"session_id"`
	Question     string     `json:"question"`
	DatasourceID string     `json:"data_source_id"`
	SQL          string     `json:"sql"`
	ChartHint    chart.Hint `json:"chart_hint"`
	CacheKey     string     `json:"cache_key"`
}

type JobEvent struct {
	At      time.Time `json:"at"`
	Type    string    `json:"type"`
	Message string    `json:"message,omitempty"`
}

type Job struct {
	ID             string              `json:"id"`
	Request        QueryRequest        `json:"request"`
	DatasourceID   string              `json:"data_source_id"`
	SelectedTables []sqlguard.TableRef `json:"selected_tables,omitempty"`
	RewrittenSQL   string              `json:"rewritten_sql,omitempty"`
	Status         string              `json:"status"`
	Error          string              `json:"error,omitempty"`
	ResultText     string              `json:"result_text,omitempty"`
	ResultTable    string              `json:"result_table,omitempty"`
	ResultCSVPath  string              `json:"result_csv_path,omitempty"`
	ResultColumns  []string            `json:"result_columns,omitempty"`
	ResultRows     [][]string          `json:"result_rows,omitempty"`
	ResultRowCount int                 `json:"result_row_count,omitempty"`
	ResultDuration int64               `json:"result_duration_ms,omitempty"`
	Events         []JobEvent          `json:"events,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type Manager struct {
	cfg           *config.Config
	ds            *datasource.Manager
	tg            *telegram.Client
	cache         *cache.FileCache
	catalog       *schema.Catalog
	queue         chan *Job
	stop          chan struct{}
	jobs          map[string]*Job
	mu            sync.Mutex
	runningByUser map[string]int
}

func NewManager(cfg *config.Config, ds *datasource.Manager, tg *telegram.Client, c *cache.FileCache, cat *schema.Catalog) *Manager {
	return &Manager{cfg: cfg, ds: ds, tg: tg, cache: c, catalog: cat, queue: make(chan *Job, cfg.Queue.BufferSize), stop: make(chan struct{}), jobs: map[string]*Job{}, runningByUser: map[string]int{}}
}

func (m *Manager) Start(ctx context.Context) {
	for i := 0; i < m.cfg.Queue.Workers; i++ {
		go m.worker(ctx, i)
	}
}
func (m *Manager) Stop() { close(m.stop) }

func (m *Manager) Submit(req QueryRequest) (*Job, error) {
	id := strings.TrimSpace(req.RequestID)
	if id == "" {
		id = randomID()
	}
	m.mu.Lock()
	if existing, ok := m.jobs[id]; ok {
		m.mu.Unlock()
		return existing, nil
	}
	now := time.Now()
	j := &Job{ID: id, Request: req, Status: "queued", CreatedAt: now, UpdatedAt: now, Events: []JobEvent{{At: now, Type: "queued", Message: "任务已进入队列"}}}
	m.jobs[id] = j
	m.mu.Unlock()
	m.persist(j)
	select {
	case m.queue <- j:
		// Do not send Telegram progress here. The worker sends the first progress
		// message synchronously before validation/execution. Sending from Submit in a
		// goroutine can race with fast failures, causing users to see "failed" before
		// "queued".
		return j, nil
	default:
		m.setStatus(j, "rejected", "queue full")
		return j, fmt.Errorf("queue full")
	}
}

func (m *Manager) worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-m.stop:
			return
		case j := <-m.queue:
			m.process(ctx, j)
		}
	}
}

func (m *Manager) process(parent context.Context, j *Job) {
	userKey := j.Request.UserID
	if userKey == "" {
		userKey = j.Request.ChatID
	}
	for !m.acquireUser(userKey) {
		select {
		case <-m.stop:
			return
		case <-time.After(300 * time.Millisecond):
		}
	}
	defer m.releaseUser(userKey)

	ctx, cancel := context.WithTimeout(parent, time.Duration(m.cfg.Queue.JobTimeoutSec)*time.Second)
	defer cancel()

	if m.cfg.Queue.NotifyOnAccepted && !m.cfg.Telegram.IsCompactResultOnly() {
		m.sendProgress(ctx, j, "已收到查询任务，任务ID："+j.ID+"，正在进行 SQL 安全校验，校验通过后会执行查询。")
	}

	m.setStatus(j, "validating", "")
	dsID, err := m.selectDatasource(j.Request.DatasourceID, j.Request.SQL)
	if err != nil {
		m.fail(ctx, j, err)
		return
	}
	dsCfg, ok := m.ds.Config(dsID)
	if !ok {
		m.fail(ctx, j, fmt.Errorf("unknown datasource: %s", dsID))
		return
	}
	checked, err := sqlguard.ValidateAndRewrite(j.Request.SQL, dsCfg.Guard, m.catalog, dsCfg.Execution.MaxRows, dsCfg.Execution.DefaultLimit)
	if err != nil {
		m.fail(ctx, j, fmt.Errorf("SQL 安全校验失败：%w", err))
		return
	}
	j.DatasourceID = dsID
	j.SelectedTables = checked.Tables
	j.RewrittenSQL = checked.SQL
	m.persist(j)

	cacheKey := j.Request.CacheKey
	if cacheKey == "" {
		cacheKey = cache.HashKey(dsID, checked.SQL)
	}
	if m.cfg.Cache.Enabled {
		if e, ok := m.cache.Get(cacheKey); ok {
			msg := buildTelegramResultMessage(checked.SQL, e.Summary)
			m.setCachedResult(j, e.Summary)
			m.sendResultMessage(ctx, j, msg)
			m.setStatus(j, "sent_cached", "")
			return
		}
	}

	m.setStatus(j, "running", "")
	if m.cfg.Queue.NotifyOnAccepted && !m.cfg.Telegram.IsCompactResultOnly() {
		m.sendProgress(ctx, j, fmt.Sprintf("SQL 安全校验通过，任务ID：%s，数据源：%s，正在执行查询。", j.ID, dsID))
	}
	result, err := m.ds.Query(ctx, dsID, checked.SQL)
	if err != nil {
		m.fail(ctx, j, fmt.Errorf("SQL 执行失败：%w", err))
		return
	}

	title := j.Request.Question
	if title == "" {
		title = j.ID
	}
	bundle := formatter.BuildText(title, result, m.cfg.Telegram.MaxInlineRows)
	resultText := strings.TrimSpace(bundle.AnswerText)
	if resultText == "" {
		resultText = strings.TrimSpace(bundle.TableText)
	}
	if resultText == "" {
		resultText = "无查询结果"
	}
	msg := buildTelegramResultMessage(checked.SQL, resultText)
	m.setResult(j, result, resultText, bundle.TableText, "")
	m.sendResultMessage(ctx, j, msg)

	var chartPath string
	if !m.cfg.Telegram.IsCompactResultOnly() && m.cfg.Telegram.SendChartSVG {
		if p, ok, err := chart.WriteSVG(m.cfg.Storage.ResultDir, j.ID, title, result, j.Request.ChartHint); err == nil && ok {
			chartPath = p
			_ = m.tg.SendDocument(ctx, j.Request.ChatID, p, "自动生成图表 SVG")
		} else if err != nil {
			log.Printf("write chart: %v", err)
		}
	}
	var csvPath string
	if m.cfg.Telegram.SendCSV {
		if p, err := formatter.WriteCSV(m.cfg.Storage.ResultDir, j.ID, result, m.cfg.Telegram.CSVCompressThreshold); err == nil {
			csvPath = p
			m.setResultCSVPath(j, csvPath)
			_ = m.tg.SendDocument(ctx, j.Request.ChatID, csvPath, "完整查询结果 CSV")
		} else {
			log.Printf("write csv: %v", err)
		}
	}
	if m.cfg.Cache.Enabled {
		_ = m.cache.Set(cache.Entry{Key: cacheKey, Summary: resultText, CSVPath: csvPath, ChartPath: chartPath})
	}
	m.setStatus(j, "sent", "")
}

func (m *Manager) selectDatasource(requested string, sqlText string) (string, error) {
	if requested != "" {
		if _, ok := m.ds.Config(requested); !ok {
			return "", fmt.Errorf("requested datasource not found: %s", requested)
		}
		return requested, nil
	}
	tables, _ := sqlguard.ExtractTables(sqlText)
	for _, rule := range m.cfg.Routing.Rules {
		if rule.Datasource == "" {
			continue
		}
		if matchesRule(rule, tables) && m.datasourceAllowsTables(rule.Datasource, tables) {
			return rule.Datasource, nil
		}
	}
	defaultID := m.ds.DefaultID()
	if m.datasourceAllowsTables(defaultID, tables) {
		return defaultID, nil
	}
	for id := range m.cfg.Datasources.Items {
		if m.datasourceAllowsTables(id, tables) {
			return id, nil
		}
	}
	return defaultID, nil
}

func (m *Manager) datasourceAllowsTables(datasourceID string, tables []sqlguard.TableRef) bool {
	dsCfg, ok := m.ds.Config(datasourceID)
	if !ok {
		return false
	}
	allowedSchemas := toSet(dsCfg.Guard.AllowedSchemas)
	allowedTables := toSet(dsCfg.Guard.AllowedTables)
	deniedSchemas := toSet(dsCfg.Guard.DeniedSchemas)
	deniedTables := toSet(dsCfg.Guard.DeniedTables)
	for _, t := range tables {
		schemaName := strings.ToLower(strings.Trim(t.Schema, " `\""))
		fullName := strings.ToLower(strings.Trim(t.Schema+"."+t.Table, " `\""))
		if schemaName != "" {
			if deniedSchemas[schemaName] {
				return false
			}
			if len(allowedSchemas) > 0 && !allowedSchemas[schemaName] {
				return false
			}
		}
		if deniedTables[fullName] {
			return false
		}
		if len(allowedTables) > 0 && !allowedTables[fullName] {
			return false
		}
	}
	return true
}

func matchesRule(rule config.RoutingRule, tables []sqlguard.TableRef) bool {
	if len(rule.MatchSchemas) == 0 && len(rule.MatchTables) == 0 {
		return false
	}
	schemas := toSet(rule.MatchSchemas)
	tbls := toSet(rule.MatchTables)
	for _, t := range tables {
		if schemas[strings.ToLower(t.Schema)] {
			return true
		}
		if tbls[strings.ToLower(t.Schema+"."+t.Table)] {
			return true
		}
	}
	return false
}

func toSet(v []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range v {
		x = strings.ToLower(strings.Trim(x, " `\""))
		if x != "" {
			m[x] = true
		}
	}
	return m
}

func (m *Manager) acquireUser(user string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningByUser[user] >= m.cfg.Queue.MaxPerUserRunning {
		return false
	}
	m.runningByUser[user]++
	return true
}
func (m *Manager) releaseUser(user string) {
	m.mu.Lock()
	if m.runningByUser[user] > 0 {
		m.runningByUser[user]--
	}
	m.mu.Unlock()
}

func (m *Manager) sendProgress(ctx context.Context, j *Job, text string) {
	if strings.TrimSpace(j.Request.ChatID) == "" {
		return
	}
	for _, chunk := range formatter.SplitText(text, m.cfg.Telegram.MessageChunkSize) {
		if err := m.tg.SendMessage(ctx, j.Request.ChatID, chunk); err != nil {
			log.Printf("send progress for job %s: %v", j.ID, err)
		}
	}
}

func (m *Manager) fail(ctx context.Context, j *Job, err error) {
	log.Printf("job %s failed: %v", j.ID, err)
	m.sendProgress(ctx, j, m.failureMessage(j, err))
	m.setStatus(j, "failed", err.Error())
}

func (m *Manager) failureMessage(j *Job, err error) string {
	sqlText := strings.TrimSpace(j.RewrittenSQL)
	if sqlText == "" {
		sqlText = strings.TrimSpace(j.Request.SQL)
	}
	return buildTelegramResultMessage(sqlText, "查询失败："+classifyError(err)+"\n错误原因："+err.Error())
}

func buildTelegramResultMessage(sqlText string, resultText string) string {
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		sqlText = "未获取到 SQL"
	}
	resultText = strings.TrimSpace(resultText)
	if resultText == "" {
		resultText = "无查询结果"
	}
	return "🔎 查询语句\n" + sqlText + "\n\n📊 查询结果\n" + resultText
}

func (m *Manager) sendResultMessage(ctx context.Context, j *Job, msg string) {
	if strings.TrimSpace(j.Request.ChatID) == "" {
		return
	}
	for _, chunk := range formatter.SplitText(msg, m.cfg.Telegram.MessageChunkSize) {
		if err := m.tg.SendMessage(ctx, j.Request.ChatID, chunk); err != nil {
			log.Printf("send message: %v", err)
		}
	}
}

func (m *Manager) setCachedResult(j *Job, resultText string) {
	m.mu.Lock()
	j.ResultText = resultText
	j.UpdatedAt = time.Now()
	j.Events = append(j.Events, JobEvent{At: j.UpdatedAt, Type: "cache_hit", Message: "命中缓存并发送结果"})
	m.mu.Unlock()
	m.persist(j)
}

func (m *Manager) setResult(j *Job, result *dbresult.Result, resultText string, tableText string, csvPath string) {
	m.mu.Lock()
	j.ResultText = resultText
	j.ResultTable = tableText
	j.ResultCSVPath = csvPath
	if result != nil {
		j.ResultColumns = append([]string(nil), result.Columns...)
		previewRows := result.Rows
		if len(previewRows) > 100 {
			previewRows = previewRows[:100]
		}
		j.ResultRows = cloneRows(previewRows)
		j.ResultRowCount = result.RowCount
		j.ResultDuration = result.DurationMS
	}
	j.UpdatedAt = time.Now()
	j.Events = append(j.Events, JobEvent{At: j.UpdatedAt, Type: "result_ready", Message: fmt.Sprintf("返回 %d 行", j.ResultRowCount)})
	m.mu.Unlock()
	m.persist(j)
}

func (m *Manager) setResultCSVPath(j *Job, csvPath string) {
	m.mu.Lock()
	j.ResultCSVPath = csvPath
	j.UpdatedAt = time.Now()
	j.Events = append(j.Events, JobEvent{At: j.UpdatedAt, Type: "csv_ready", Message: csvPath})
	m.mu.Unlock()
	m.persist(j)
}

func cloneRows(in [][]string) [][]string {
	out := make([][]string, len(in))
	for i, r := range in {
		out[i] = append([]string(nil), r...)
	}
	return out
}

func classifyError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unknown column") || strings.Contains(msg, "unknown table") || strings.Contains(msg, "no table found") || strings.Contains(msg, "table not found"):
		return "SQL 结构错误：表或字段不存在"
	case strings.Contains(msg, "sql 安全校验失败") || strings.Contains(msg, "only select") || strings.Contains(msg, "dangerous") || strings.Contains(msg, "denied"):
		return "SQL 安全校验失败"
	case strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "timeout"):
		return "SQL 执行超时"
	default:
		return "SQL 执行失败"
	}
}

func (m *Manager) setStatus(j *Job, status, errText string) {
	m.mu.Lock()
	j.Status = status
	j.Error = errText
	j.UpdatedAt = time.Now()
	message := status
	if strings.TrimSpace(errText) != "" {
		message = errText
	}
	j.Events = append(j.Events, JobEvent{At: j.UpdatedAt, Type: status, Message: message})
	m.mu.Unlock()
	m.persist(j)
}

func (m *Manager) persist(j *Job) {
	_ = os.MkdirAll(m.cfg.Storage.JobDir, 0755)
	b, _ := json.MarshalIndent(j, "", "  ")
	_ = os.WriteFile(filepath.Join(m.cfg.Storage.JobDir, j.ID+".json"), b, 0644)
}

func randomID() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
