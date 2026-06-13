package config

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Admin       AdminConfig       `yaml:"admin"`
	Datasources DatasourcesConfig `yaml:"datasources"`
	Routing     RoutingConfig     `yaml:"routing"`
	Queue       QueueConfig       `yaml:"queue"`
	Telegram    TelegramConfig    `yaml:"telegram"`
	Storage     StorageConfig     `yaml:"storage"`
	Cache       CacheConfig       `yaml:"cache"`
	Schema      SchemaConfig      `yaml:"schema"`
}

// AdminConfig controls the optional management UI auth, SSO metadata and schema export defaults.
// Auth and SSO are intentionally disabled by default so existing no-auth OpenClaw
// integrations keep working unless operators explicitly enable them.
type AdminConfig struct {
	Auth         AdminAuthConfig         `yaml:"auth"`
	SSO          AdminSSOConfig          `yaml:"sso"`
	Users        AdminUsersConfig        `yaml:"users"`
	SchemaExport AdminSchemaExportConfig `yaml:"schema_export"`
}

type AdminAuthConfig struct {
	Enabled         bool   `yaml:"enabled"`
	SessionTTLHours int    `yaml:"session_ttl_hours"`
	CookieName      string `yaml:"cookie_name"`
	AdminTokenEnv   string `yaml:"admin_token_env"`
}

type AdminSSOConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	IssuerURL    string   `yaml:"issuer_url" json:"issuer_url"`
	ClientID     string   `yaml:"client_id" json:"client_id"`
	ClientSecret string   `yaml:"client_secret" json:"-"`
	RedirectURL  string   `yaml:"redirect_url" json:"redirect_url"`
	Scopes       string   `yaml:"scopes" json:"scopes"`
	AdminUsers   []string `yaml:"admin_users" json:"admin_users"`
	AdminRoles   []string `yaml:"admin_roles" json:"admin_roles"`
	UserRoles    []string `yaml:"user_roles" json:"user_roles"`
}

type AdminUsersConfig struct {
	File string `yaml:"file"`
}

type AdminSchemaExportConfig struct {
	Dir                  string   `yaml:"dir"`
	MaxRows              int      `yaml:"max_rows"`
	IncludeSystemSchemas bool     `yaml:"include_system_schemas"`
	SystemSchemas        []string `yaml:"system_schemas"`
}

type ServerConfig struct {
	Addr                 string `yaml:"addr"`
	ReadHeaderTimeoutSec int    `yaml:"read_header_timeout_sec"`
	ReadTimeoutSec       int    `yaml:"read_timeout_sec"`
	WriteTimeoutSec      int    `yaml:"write_timeout_sec"`
	IdleTimeoutSec       int    `yaml:"idle_timeout_sec"`
}

type DatasourcesConfig struct {
	Default string                      `yaml:"default"`
	Items   map[string]DataSourceConfig `yaml:"items"`
}

type DataSourceConfig struct {
	Type             string            `yaml:"type"`
	Driver           string            `yaml:"driver"`
	Description      string            `yaml:"description"`
	Hosts            []HostConfig      `yaml:"hosts"`
	User             string            `yaml:"user"`
	Password         string            `yaml:"password"`
	UsernameEnv      string            `yaml:"username_env"`
	PasswordEnv      string            `yaml:"password_env"`
	Database         string            `yaml:"database"`
	Charset          string            `yaml:"charset"`
	Params           map[string]string `yaml:"params"`
	Pool             PoolConfig        `yaml:"pool"`
	Execution        ExecutionConfig   `yaml:"execution"`
	Guard            GuardConfig       `yaml:"guard"`
	PreQuerySettings []string          `yaml:"pre_query_settings"`
}

type HostConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type PoolConfig struct {
	MaxOpenConns       int `yaml:"max_open_conns"`
	MaxIdleConns       int `yaml:"max_idle_conns"`
	ConnMaxLifetimeSec int `yaml:"conn_max_lifetime_sec"`
	ConnMaxIdleTimeSec int `yaml:"conn_max_idle_time_sec"`
}

type ExecutionConfig struct {
	MaxConcurrency       int `yaml:"max_concurrency"`
	QueryTimeoutSec      int `yaml:"query_timeout_sec"`
	MaxRows              int `yaml:"max_rows"`
	DefaultLimit         int `yaml:"default_limit"`
	HostFailureThreshold int `yaml:"host_failure_threshold"`
	HostCooldownSec      int `yaml:"host_cooldown_sec"`
}

type GuardConfig struct {
	MaxSQLBytes                  int      `yaml:"max_sql_bytes"`
	RequireSchemaQualifiedTables bool     `yaml:"require_schema_qualified_tables"`
	RequireLimit                 bool     `yaml:"require_limit"`
	AppendLimitIfMissing         bool     `yaml:"append_limit_if_missing"`
	EnforceKnownTables           bool     `yaml:"enforce_known_tables"`
	AllowCrossSchemaJoin         bool     `yaml:"allow_cross_schema_join"`
	AllowSubquery                bool     `yaml:"allow_subquery"`
	AllowExplain                 bool     `yaml:"allow_explain"`
	DenyRawDetailTablesByDefault bool     `yaml:"deny_raw_detail_tables_by_default"`
	AllowedSchemas               []string `yaml:"allowed_schemas"`
	DeniedSchemas                []string `yaml:"denied_schemas"`
	AllowedTables                []string `yaml:"allowed_tables"`
	DeniedTables                 []string `yaml:"denied_tables"`
	LargeTables                  []string `yaml:"large_tables"`
	RequireWhereForTables        []string `yaml:"require_where_for_tables"`
	RequireTimeForTables         []string `yaml:"require_time_for_tables"`
	TimeColumnHints              []string `yaml:"time_column_hints"`
	DangerousFunctions           []string `yaml:"dangerous_functions"`
}

type RoutingConfig struct {
	Rules []RoutingRule `yaml:"rules"`
}

type RoutingRule struct {
	Name         string   `yaml:"name"`
	MatchSchemas []string `yaml:"match_schemas"`
	MatchTables  []string `yaml:"match_tables"`
	Datasource   string   `yaml:"datasource"`
}

type QueueConfig struct {
	Workers           int  `yaml:"workers"`
	BufferSize        int  `yaml:"buffer_size"`
	MaxPerUserRunning int  `yaml:"max_per_user_running"`
	NotifyOnAccepted  bool `yaml:"notify_on_accepted"`
	JobTimeoutSec     int  `yaml:"job_timeout_sec"`
}

type TelegramConfig struct {
	BotToken             string `yaml:"bot_token"`
	BaseURL              string `yaml:"base_url"`
	MessageChunkSize     int    `yaml:"message_chunk_size"`
	MaxInlineRows        int    `yaml:"max_inline_rows"`
	CSVCompressThreshold int    `yaml:"csv_compress_threshold_bytes"`
	// CompactResultOnly defaults to true when omitted. In compact mode Telegram
	// notifications only include the executed SQL and the formatted result.
	CompactResultOnly   *bool `yaml:"compact_result_only"`
	SendCSV             bool  `yaml:"send_csv"`
	SendChartSVG        bool  `yaml:"send_chart_svg"`
	DisableNotification bool  `yaml:"disable_notification"`
}

func (t TelegramConfig) IsCompactResultOnly() bool {
	return t.CompactResultOnly == nil || *t.CompactResultOnly
}

type StorageConfig struct {
	DataDir   string `yaml:"data_dir"`
	CacheDir  string `yaml:"cache_dir"`
	ResultDir string `yaml:"result_dir"`
	JobDir    string `yaml:"job_dir"`
}

type CacheConfig struct {
	Enabled    bool `yaml:"enabled"`
	TTLSeconds int  `yaml:"ttl_seconds"`
}

type SchemaConfig struct {
	CatalogPath string `yaml:"catalog_path"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	expanded := os.ExpandEnv(string(b))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(c *Config) {
	if c.Admin.Auth.SessionTTLHours <= 0 {
		c.Admin.Auth.SessionTTLHours = 12
	}
	if c.Admin.Auth.CookieName == "" {
		c.Admin.Auth.CookieName = "openclaw_admin_session"
	}
	if c.Admin.Auth.AdminTokenEnv == "" {
		c.Admin.Auth.AdminTokenEnv = "OPENCLAW_ADMIN_TOKEN"
	}
	if c.Admin.SSO.Scopes == "" {
		c.Admin.SSO.Scopes = "openid profile email"
	}
	if len(c.Admin.SSO.AdminRoles) == 0 {
		c.Admin.SSO.AdminRoles = []string{"admin", "openclaw-admin"}
	}
	if len(c.Admin.SSO.UserRoles) == 0 {
		c.Admin.SSO.UserRoles = []string{"user", "openclaw-user"}
	}
	if c.Admin.Users.File == "" {
		c.Admin.Users.File = c.Storage.DataDir + "/admin/users.json"
	}
	if c.Admin.SchemaExport.Dir == "" {
		c.Admin.SchemaExport.Dir = c.Storage.DataDir + "/schema_exports"
	}
	if c.Admin.SchemaExport.MaxRows <= 0 {
		c.Admin.SchemaExport.MaxRows = 200000
	}
	if len(c.Admin.SchemaExport.SystemSchemas) == 0 {
		c.Admin.SchemaExport.SystemSchemas = []string{"information_schema", "mysql", "performance_schema", "sys", "__internal_schema"}
	}
	if c.Server.Addr == "" {
		c.Server.Addr = ":8088"
	}
	if c.Server.ReadHeaderTimeoutSec <= 0 {
		c.Server.ReadHeaderTimeoutSec = 5
	}
	if c.Server.ReadTimeoutSec <= 0 {
		c.Server.ReadTimeoutSec = 20
	}
	if c.Server.WriteTimeoutSec <= 0 {
		c.Server.WriteTimeoutSec = 20
	}
	if c.Server.IdleTimeoutSec <= 0 {
		c.Server.IdleTimeoutSec = 60
	}
	if c.Queue.Workers <= 0 {
		c.Queue.Workers = 6
	}
	if c.Queue.BufferSize <= 0 {
		c.Queue.BufferSize = 500
	}
	if c.Queue.MaxPerUserRunning <= 0 {
		c.Queue.MaxPerUserRunning = 1
	}
	if c.Queue.JobTimeoutSec <= 0 {
		c.Queue.JobTimeoutSec = 120
	}
	if c.Telegram.BaseURL == "" {
		c.Telegram.BaseURL = "https://api.telegram.org"
	}
	if c.Telegram.MessageChunkSize <= 0 {
		c.Telegram.MessageChunkSize = 3500
	}
	if c.Telegram.MaxInlineRows <= 0 {
		c.Telegram.MaxInlineRows = 30
	}
	if c.Telegram.CSVCompressThreshold <= 0 {
		c.Telegram.CSVCompressThreshold = 512 * 1024
	}
	if c.Storage.DataDir == "" {
		c.Storage.DataDir = "./data"
	}
	if c.Storage.CacheDir == "" {
		c.Storage.CacheDir = c.Storage.DataDir + "/cache"
	}
	if c.Storage.ResultDir == "" {
		c.Storage.ResultDir = c.Storage.DataDir + "/results"
	}
	if c.Storage.JobDir == "" {
		c.Storage.JobDir = c.Storage.DataDir + "/jobs"
	}
	if c.Admin.Users.File == "" || strings.HasPrefix(c.Admin.Users.File, "/admin/") {
		c.Admin.Users.File = c.Storage.DataDir + "/admin/users.json"
	}
	if c.Admin.SchemaExport.Dir == "" || strings.HasPrefix(c.Admin.SchemaExport.Dir, "/schema_exports") {
		c.Admin.SchemaExport.Dir = c.Storage.DataDir + "/schema_exports"
	}
	if c.Cache.TTLSeconds <= 0 {
		c.Cache.TTLSeconds = 300
	}

	for id, ds := range c.Datasources.Items {
		if ds.Type == "" {
			ds.Type = "mysql"
		}
		if ds.Driver == "" {
			ds.Driver = "mysql"
		}
		if ds.Charset == "" {
			ds.Charset = "utf8mb4"
		}
		if ds.UsernameEnv != "" && ds.User == "" {
			ds.User = os.Getenv(ds.UsernameEnv)
		}
		if ds.PasswordEnv != "" && ds.Password == "" {
			ds.Password = os.Getenv(ds.PasswordEnv)
		}
		if ds.Pool.MaxOpenConns <= 0 {
			ds.Pool.MaxOpenConns = 5
		}
		if ds.Pool.MaxIdleConns <= 0 {
			ds.Pool.MaxIdleConns = 2
		}
		if ds.Pool.ConnMaxLifetimeSec <= 0 {
			ds.Pool.ConnMaxLifetimeSec = 300
		}
		if ds.Pool.ConnMaxIdleTimeSec <= 0 {
			ds.Pool.ConnMaxIdleTimeSec = 120
		}
		if ds.Execution.MaxConcurrency <= 0 {
			ds.Execution.MaxConcurrency = 2
		}
		if ds.Execution.QueryTimeoutSec <= 0 {
			ds.Execution.QueryTimeoutSec = 45
		}
		if ds.Execution.MaxRows <= 0 {
			ds.Execution.MaxRows = 1000
		}
		if ds.Execution.DefaultLimit <= 0 {
			ds.Execution.DefaultLimit = 1000
		}
		if ds.Execution.HostFailureThreshold <= 0 {
			ds.Execution.HostFailureThreshold = 3
		}
		if ds.Execution.HostCooldownSec <= 0 {
			ds.Execution.HostCooldownSec = 30
		}
		if ds.Guard.MaxSQLBytes <= 0 {
			ds.Guard.MaxSQLBytes = 32 * 1024
		}
		if len(ds.Guard.TimeColumnHints) == 0 {
			ds.Guard.TimeColumnHints = []string{"dt", "day", "date", "time", "create_time", "update_time", "record_date", "user_event_date", "event_day_local", "utc_event_5min"}
		}
		if len(ds.Guard.DangerousFunctions) == 0 {
			ds.Guard.DangerousFunctions = []string{"sleep", "benchmark", "load_file", "into outfile", "into dumpfile"}
		}
		c.Datasources.Items[id] = ds
	}
}

func validate(c *Config) error {
	if len(c.Datasources.Items) == 0 {
		return errors.New("datasources.items is required")
	}
	if c.Datasources.Default == "" {
		ids := make([]string, 0, len(c.Datasources.Items))
		for id := range c.Datasources.Items {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		c.Datasources.Default = ids[0]
	}
	if _, ok := c.Datasources.Items[c.Datasources.Default]; !ok {
		return fmt.Errorf("datasources.default %q is not defined", c.Datasources.Default)
	}
	for id, ds := range c.Datasources.Items {
		if strings.TrimSpace(id) == "" {
			return errors.New("datasource id cannot be empty")
		}
		if strings.TrimSpace(ds.User) == "" {
			return fmt.Errorf("datasource %s user is required", id)
		}
		if len(ds.Hosts) == 0 {
			return fmt.Errorf("datasource %s hosts is required", id)
		}
		for _, h := range ds.Hosts {
			if strings.TrimSpace(h.Host) == "" || h.Port <= 0 {
				return fmt.Errorf("datasource %s has invalid host", id)
			}
		}
		if ds.Execution.DefaultLimit > ds.Execution.MaxRows {
			return fmt.Errorf("datasource %s execution.default_limit must be <= max_rows", id)
		}
	}
	for _, r := range c.Routing.Rules {
		if r.Datasource == "" {
			return fmt.Errorf("routing rule %s datasource is required", r.Name)
		}
		if _, ok := c.Datasources.Items[r.Datasource]; !ok {
			return fmt.Errorf("routing rule %s datasource %q not defined", r.Name, r.Datasource)
		}
	}
	if strings.TrimSpace(c.Telegram.BotToken) == "" {
		return errors.New("telegram.bot_token is required")
	}
	return nil
}
