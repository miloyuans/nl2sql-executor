package datasource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/dbresult"
)

type Manager struct {
	cfg     config.DatasourcesConfig
	sources map[string]*Source
}

type Source struct {
	ID         string
	Config     config.DataSourceConfig
	hosts      []*hostPool
	sem        chan struct{}
	roundRobin int
	mu         sync.Mutex
}

type hostPool struct {
	addr       string
	db         *sql.DB
	mu         sync.Mutex
	failCount  int
	coolUntil  time.Time
	lastErr    string
	lastOKTime time.Time
}

type PublicStatus struct {
	ID          string       `json:"id"`
	Description string       `json:"description"`
	Default     bool         `json:"default"`
	Hosts       []HostStatus `json:"hosts"`
	MaxRows     int          `json:"max_rows"`
	Concurrency int          `json:"max_concurrency"`
}

type HostStatus struct {
	Addr       string    `json:"addr"`
	Cooling    bool      `json:"cooling"`
	CoolUntil  time.Time `json:"cool_until,omitempty"`
	FailCount  int       `json:"fail_count"`
	LastError  string    `json:"last_error,omitempty"`
	LastOKTime time.Time `json:"last_ok_time,omitempty"`
}

func NewManager(cfg config.DatasourcesConfig) (*Manager, error) {
	m := &Manager{cfg: cfg, sources: map[string]*Source{}}
	ids := make([]string, 0, len(cfg.Items))
	for id := range cfg.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		src, err := newSource(id, cfg.Items[id])
		if err != nil {
			return nil, err
		}
		m.sources[id] = src
	}
	return m, nil
}

func newSource(id string, cfg config.DataSourceConfig) (*Source, error) {
	s := &Source{ID: id, Config: cfg, sem: make(chan struct{}, cfg.Execution.MaxConcurrency)}
	for _, h := range cfg.Hosts {
		addr := fmt.Sprintf("%s:%d", h.Host, h.Port)
		dsn := buildDSN(cfg, addr)
		db, err := sql.Open(cfg.Driver, dsn)
		if err != nil {
			return nil, fmt.Errorf("datasource %s open %s: %w", id, addr, err)
		}
		db.SetMaxOpenConns(cfg.Pool.MaxOpenConns)
		db.SetMaxIdleConns(cfg.Pool.MaxIdleConns)
		db.SetConnMaxLifetime(time.Duration(cfg.Pool.ConnMaxLifetimeSec) * time.Second)
		db.SetConnMaxIdleTime(time.Duration(cfg.Pool.ConnMaxIdleTimeSec) * time.Second)
		s.hosts = append(s.hosts, &hostPool{addr: addr, db: db})
	}
	return s, nil
}

func buildDSN(cfg config.DataSourceConfig, addr string) string {
	mc := mysql.NewConfig()
	mc.User = cfg.User
	mc.Passwd = cfg.Password
	mc.Net = "tcp"
	mc.Addr = addr
	mc.DBName = cfg.Database
	mc.ParseTime = true
	mc.Loc = time.Local
	mc.Params = map[string]string{}
	if cfg.Charset != "" {
		mc.Params["charset"] = cfg.Charset
	}
	if len(cfg.Params) > 0 {
		for k, v := range cfg.Params {
			mc.Params[k] = v
		}
	} else {
		mc.Params["timeout"] = "10s"
		mc.Params["readTimeout"] = "60s"
		mc.Params["writeTimeout"] = "60s"
	}
	return mc.FormatDSN()
}

func (m *Manager) DefaultID() string { return m.cfg.Default }

func (m *Manager) Get(id string) (*Source, bool) {
	if id == "" {
		id = m.cfg.Default
	}
	s, ok := m.sources[id]
	return s, ok
}

func (m *Manager) Config(id string) (config.DataSourceConfig, bool) {
	s, ok := m.Get(id)
	if !ok {
		return config.DataSourceConfig{}, false
	}
	return s.Config, true
}

func (m *Manager) Query(ctx context.Context, id string, sqlText string) (*dbresult.Result, error) {
	s, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("unknown datasource: %s", id)
	}
	return s.Query(ctx, sqlText)
}

func (s *Source) Query(ctx context.Context, sqlText string) (*dbresult.Result, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	qctx, cancel := context.WithTimeout(ctx, time.Duration(s.Config.Execution.QueryTimeoutSec)*time.Second)
	defer cancel()

	candidates := s.orderedHosts()
	if len(candidates) == 0 {
		return nil, errors.New("datasource has no hosts")
	}
	var firstErr error
	for _, hp := range candidates {
		if hp.isCooling() {
			continue
		}
		res, err := s.queryOnHost(qctx, hp, sqlText)
		if err == nil {
			hp.markOK()
			res.Datasource = s.ID
			res.Host = hp.addr
			return res, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		hp.markFail(err, s.Config.Execution.HostFailureThreshold, time.Duration(s.Config.Execution.HostCooldownSec)*time.Second)
		log.Printf("datasource=%s host=%s query failed: %v", s.ID, hp.addr, err)
		if !isRetryableDBError(err) {
			return nil, err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, errors.New("all datasource hosts are cooling down")
}

func (s *Source) queryOnHost(ctx context.Context, hp *hostPool, sqlText string) (*dbresult.Result, error) {
	start := time.Now()
	conn, err := hp.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	for _, setting := range s.Config.PreQuerySettings {
		setting = strings.TrimSpace(setting)
		if setting == "" {
			continue
		}
		if _, err := conn.ExecContext(ctx, setting); err != nil {
			log.Printf("pre query setting failed datasource=%s host=%s setting=%q err=%v", s.ID, hp.addr, setting, err)
		}
	}
	rows, err := conn.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := &dbresult.Result{Columns: cols, Rows: make([][]string, 0, 64)}
	raw := make([]sql.RawBytes, len(cols))
	dest := make([]any, len(cols))
	for i := range raw {
		dest[i] = &raw[i]
	}
	for rows.Next() {
		if out.RowCount >= s.Config.Execution.MaxRows {
			out.Truncated = true
			break
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		r := make([]string, len(cols))
		for i, v := range raw {
			if v == nil {
				r[i] = "NULL"
			} else {
				r[i] = string(v)
			}
		}
		out.Rows = append(out.Rows, r)
		out.RowCount++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.DurationMS = time.Since(start).Milliseconds()
	return out, nil
}

func (s *Source) orderedHosts() []*hostPool {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.hosts)
	if n == 0 {
		return nil
	}
	start := s.roundRobin % n
	s.roundRobin = (s.roundRobin + 1) % n
	ordered := make([]*hostPool, 0, n)
	for i := 0; i < n; i++ {
		ordered = append(ordered, s.hosts[(start+i)%n])
	}
	if n > 2 && rand.Intn(3) == 0 {
		rand.Shuffle(n, func(i, j int) { ordered[i], ordered[j] = ordered[j], ordered[i] })
	}
	return ordered
}

func (h *hostPool) isCooling() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.coolUntil.IsZero() && time.Now().Before(h.coolUntil)
}

func (h *hostPool) markOK() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failCount = 0
	h.coolUntil = time.Time{}
	h.lastErr = ""
	h.lastOKTime = time.Now()
}

func (h *hostPool) markFail(err error, threshold int, cooldown time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failCount++
	h.lastErr = err.Error()
	if h.failCount >= threshold {
		h.coolUntil = time.Now().Add(cooldown)
	}
}

func isRetryableDBError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	s := strings.ToLower(err.Error())
	parts := []string{"bad connection", "connection refused", "connection reset", "broken pipe", "i/o timeout", "timeout", "invalid connection", "no such host", "eof"}
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func (m *Manager) Status() []PublicStatus {
	ids := make([]string, 0, len(m.sources))
	for id := range m.sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]PublicStatus, 0, len(ids))
	for _, id := range ids {
		s := m.sources[id]
		st := PublicStatus{ID: id, Description: s.Config.Description, Default: id == m.cfg.Default, MaxRows: s.Config.Execution.MaxRows, Concurrency: s.Config.Execution.MaxConcurrency}
		for _, hp := range s.hosts {
			hp.mu.Lock()
			st.Hosts = append(st.Hosts, HostStatus{Addr: hp.addr, Cooling: !hp.coolUntil.IsZero() && time.Now().Before(hp.coolUntil), CoolUntil: hp.coolUntil, FailCount: hp.failCount, LastError: hp.lastErr, LastOKTime: hp.lastOKTime})
			hp.mu.Unlock()
		}
		out = append(out, st)
	}
	return out
}

func (m *Manager) Ping(ctx context.Context) map[string]map[string]string {
	out := map[string]map[string]string{}
	for id, s := range m.sources {
		out[id] = map[string]string{}
		for _, hp := range s.hosts {
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := hp.db.PingContext(pctx)
			cancel()
			if err != nil {
				out[id][hp.addr] = err.Error()
			} else {
				out[id][hp.addr] = "ok"
				hp.markOK()
			}
		}
	}
	return out
}

func (m *Manager) Close() error {
	var errs []string
	for id, s := range m.sources {
		for _, hp := range s.hosts {
			if err := hp.db.Close(); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", id, hp.addr, err))
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
