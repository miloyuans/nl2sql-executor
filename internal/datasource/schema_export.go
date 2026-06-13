package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

type SchemaExportOptions struct {
	IncludeSystemSchemas bool
	SystemSchemas        []string
	MaxRows              int
}

type SchemaExport struct {
	DatasourceID string              `json:"datasource_id"`
	Host         string              `json:"host"`
	GeneratedAt  time.Time           `json:"generated_at"`
	Databases    []DatabaseMetadata  `json:"databases"`
	Tables       []TableMetadata     `json:"tables"`
	Columns      []ColumnMetadata    `json:"columns"`
	Indexes      []IndexMetadata     `json:"indexes"`
	Views        []ViewMetadata      `json:"views"`
	Errors       []string            `json:"errors,omitempty"`
	Summary      SchemaExportSummary `json:"summary"`
}

type SchemaExportSummary struct {
	DatabaseCount int `json:"database_count"`
	TableCount    int `json:"table_count"`
	ViewCount     int `json:"view_count"`
	ColumnCount   int `json:"column_count"`
	IndexCount    int `json:"index_count"`
}

type DatabaseMetadata struct {
	SchemaName       string `json:"schema_name"`
	DefaultCharset   string `json:"default_charset,omitempty"`
	DefaultCollation string `json:"default_collation,omitempty"`
}

type TableMetadata struct {
	SchemaName   string `json:"schema_name"`
	TableName    string `json:"table_name"`
	TableType    string `json:"table_type"`
	Engine       string `json:"engine,omitempty"`
	TableRows    string `json:"table_rows,omitempty"`
	CreateTime   string `json:"create_time,omitempty"`
	UpdateTime   string `json:"update_time,omitempty"`
	TableComment string `json:"table_comment,omitempty"`
}

type ColumnMetadata struct {
	SchemaName    string `json:"schema_name"`
	TableName     string `json:"table_name"`
	ColumnName    string `json:"column_name"`
	Ordinal       string `json:"ordinal_position"`
	ColumnDefault string `json:"column_default,omitempty"`
	Nullable      string `json:"is_nullable,omitempty"`
	DataType      string `json:"data_type,omitempty"`
	ColumnType    string `json:"column_type,omitempty"`
	ColumnKey     string `json:"column_key,omitempty"`
	Extra         string `json:"extra,omitempty"`
	Comment       string `json:"column_comment,omitempty"`
}

type IndexMetadata struct {
	SchemaName string `json:"schema_name"`
	TableName  string `json:"table_name"`
	IndexName  string `json:"index_name"`
	NonUnique  string `json:"non_unique"`
	SeqInIndex string `json:"seq_in_index"`
	ColumnName string `json:"column_name"`
	IndexType  string `json:"index_type,omitempty"`
	Comment    string `json:"index_comment,omitempty"`
}

type ViewMetadata struct {
	SchemaName     string `json:"schema_name"`
	ViewName       string `json:"view_name"`
	CheckOption    string `json:"check_option,omitempty"`
	IsUpdatable    string `json:"is_updatable,omitempty"`
	SecurityType   string `json:"security_type,omitempty"`
	ViewDefinition string `json:"view_definition,omitempty"`
}

func (m *Manager) ExportSchema(ctx context.Context, id string, opt SchemaExportOptions) (*SchemaExport, error) {
	s, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("unknown datasource: %s", id)
	}
	return s.ExportSchema(ctx, opt)
}

func (s *Source) ExportSchema(ctx context.Context, opt SchemaExportOptions) (*SchemaExport, error) {
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
		return nil, fmt.Errorf("datasource has no hosts")
	}
	var firstErr error
	for _, hp := range candidates {
		if hp.isCooling() {
			continue
		}
		exp, err := s.exportSchemaOnHost(qctx, hp, opt)
		if err == nil {
			hp.markOK()
			return exp, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		hp.markFail(err, s.Config.Execution.HostFailureThreshold, time.Duration(s.Config.Execution.HostCooldownSec)*time.Second)
		if !isRetryableDBError(err) {
			return nil, err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("all datasource hosts are cooling down")
}

func (s *Source) exportSchemaOnHost(ctx context.Context, hp *hostPool, opt SchemaExportOptions) (*SchemaExport, error) {
	conn, err := hp.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	exp := &SchemaExport{DatasourceID: s.ID, Host: hp.addr, GeneratedAt: time.Now()}
	maxRows := opt.MaxRows
	if maxRows <= 0 {
		maxRows = 200000
	}
	skip := map[string]struct{}{}
	for _, v := range opt.SystemSchemas {
		if strings.TrimSpace(v) != "" {
			skip[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
		}
	}
	where := ""
	if !opt.IncludeSystemSchemas && len(skip) > 0 {
		parts := make([]string, 0, len(skip))
		for v := range skip {
			parts = append(parts, "'"+strings.ReplaceAll(v, "'", "''")+"'")
		}
		sort.Strings(parts)
		where = " WHERE lower(table_schema) NOT IN (" + strings.Join(parts, ",") + ")"
	}
	schemaWhere := strings.Replace(where, "table_schema", "schema_name", 1)

	if rows, err := queryAll(ctx, conn, "SELECT schema_name, default_character_set_name, default_collation_name FROM information_schema.schemata"+schemaWhere+" ORDER BY schema_name", maxRows); err == nil {
		for _, r := range rows {
			exp.Databases = append(exp.Databases, DatabaseMetadata{SchemaName: getCell(r, 0), DefaultCharset: getCell(r, 1), DefaultCollation: getCell(r, 2)})
		}
	} else {
		exp.Errors = append(exp.Errors, "schemata: "+err.Error())
	}
	if rows, err := queryAll(ctx, conn, "SELECT table_schema, table_name, table_type, engine, table_rows, create_time, update_time, table_comment FROM information_schema.tables"+where+" ORDER BY table_schema, table_name", maxRows); err == nil {
		for _, r := range rows {
			exp.Tables = append(exp.Tables, TableMetadata{SchemaName: getCell(r, 0), TableName: getCell(r, 1), TableType: getCell(r, 2), Engine: getCell(r, 3), TableRows: getCell(r, 4), CreateTime: getCell(r, 5), UpdateTime: getCell(r, 6), TableComment: getCell(r, 7)})
		}
	} else {
		exp.Errors = append(exp.Errors, "tables: "+err.Error())
	}
	if rows, err := queryAll(ctx, conn, "SELECT table_schema, table_name, column_name, ordinal_position, column_default, is_nullable, data_type, column_type, column_key, extra, column_comment FROM information_schema.columns"+where+" ORDER BY table_schema, table_name, ordinal_position", maxRows); err == nil {
		for _, r := range rows {
			exp.Columns = append(exp.Columns, ColumnMetadata{SchemaName: getCell(r, 0), TableName: getCell(r, 1), ColumnName: getCell(r, 2), Ordinal: getCell(r, 3), ColumnDefault: getCell(r, 4), Nullable: getCell(r, 5), DataType: getCell(r, 6), ColumnType: getCell(r, 7), ColumnKey: getCell(r, 8), Extra: getCell(r, 9), Comment: getCell(r, 10)})
		}
	} else {
		exp.Errors = append(exp.Errors, "columns: "+err.Error())
	}
	if rows, err := queryAll(ctx, conn, "SELECT table_schema, table_name, index_name, non_unique, seq_in_index, column_name, index_type, index_comment FROM information_schema.statistics"+where+" ORDER BY table_schema, table_name, index_name, seq_in_index", maxRows); err == nil {
		for _, r := range rows {
			exp.Indexes = append(exp.Indexes, IndexMetadata{SchemaName: getCell(r, 0), TableName: getCell(r, 1), IndexName: getCell(r, 2), NonUnique: getCell(r, 3), SeqInIndex: getCell(r, 4), ColumnName: getCell(r, 5), IndexType: getCell(r, 6), Comment: getCell(r, 7)})
		}
	} else {
		exp.Errors = append(exp.Errors, "statistics: "+err.Error())
	}
	if rows, err := queryAll(ctx, conn, "SELECT table_schema, table_name, check_option, is_updatable, security_type, view_definition FROM information_schema.views"+where+" ORDER BY table_schema, table_name", maxRows); err == nil {
		for _, r := range rows {
			exp.Views = append(exp.Views, ViewMetadata{SchemaName: getCell(r, 0), ViewName: getCell(r, 1), CheckOption: getCell(r, 2), IsUpdatable: getCell(r, 3), SecurityType: getCell(r, 4), ViewDefinition: getCell(r, 5)})
		}
	} else {
		exp.Errors = append(exp.Errors, "views: "+err.Error())
	}
	exp.Summary = SchemaExportSummary{DatabaseCount: len(exp.Databases), TableCount: len(exp.Tables), ViewCount: len(exp.Views), ColumnCount: len(exp.Columns), IndexCount: len(exp.Indexes)}
	if len(exp.Errors) > 0 && len(exp.Databases)+len(exp.Tables)+len(exp.Columns)+len(exp.Indexes)+len(exp.Views) == 0 {
		return exp, fmt.Errorf(strings.Join(exp.Errors, "; "))
	}
	return exp, nil
}

func queryAll(ctx context.Context, conn *sql.Conn, sqlText string, maxRows int) ([][]string, error) {
	rows, err := conn.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	raw := make([]sql.RawBytes, len(cols))
	dest := make([]any, len(cols))
	for i := range raw {
		dest[i] = &raw[i]
	}
	out := make([][]string, 0, 1024)
	for rows.Next() {
		if maxRows > 0 && len(out) >= maxRows {
			break
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		r := make([]string, len(cols))
		for i, v := range raw {
			if v == nil {
				r[i] = ""
			} else {
				r[i] = string(v)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func getCell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return row[idx]
}
