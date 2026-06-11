package sqlguard

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/schema"
)

type TableRef struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Full   string `json:"full"`
}

type CheckedSQL struct {
	SQL    string     `json:"sql"`
	Tables []TableRef `json:"tables"`
}

var tableRe = regexp.MustCompile(`(?i)\b(?:from|join)\s+((?:` + "`[^`]+`" + `|"[^"]+"|[a-zA-Z_][\w\-]*)(?:\s*\.\s*(?:` + "`[^`]+`" + `|"[^"]+"|[a-zA-Z_][\w\-]*))?)`)
var limitRe = regexp.MustCompile(`(?i)\blimit\s+\d+\b`)
var whereRe = regexp.MustCompile(`(?i)\bwhere\b`)
var selectStartRe = regexp.MustCompile(`(?is)^\s*(?:explain\s+)?(?:select|with)\b`)

func ValidateAndRewrite(sqlText string, guard config.GuardConfig, cat *schema.Catalog, maxRows int, defaultLimit int) (CheckedSQL, error) {
	if strings.TrimSpace(sqlText) == "" {
		return CheckedSQL{}, fmt.Errorf("empty sql")
	}
	if guard.MaxSQLBytes > 0 && len([]byte(sqlText)) > guard.MaxSQLBytes {
		return CheckedSQL{}, fmt.Errorf("sql too large")
	}
	clean := stripComments(sqlText)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return CheckedSQL{}, fmt.Errorf("empty sql after removing comments")
	}
	if !guard.AllowExplain && regexp.MustCompile(`(?i)^\s*explain\b`).MatchString(clean) {
		return CheckedSQL{}, fmt.Errorf("EXPLAIN is not allowed")
	}
	if !selectStartRe.MatchString(clean) {
		return CheckedSQL{}, fmt.Errorf("only SELECT or WITH ... SELECT is allowed")
	}
	if hasMultipleStatements(clean) {
		return CheckedSQL{}, fmt.Errorf("multiple statements are not allowed")
	}
	lower := strings.ToLower(clean)
	for _, kw := range []string{"insert", "update", "delete", "drop", "alter", "truncate", "create", "replace", "merge", "grant", "revoke", "load", "outfile", "dumpfile", "call", "set", "use"} {
		if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`).MatchString(lower) {
			return CheckedSQL{}, fmt.Errorf("dangerous keyword is not allowed: %s", kw)
		}
	}
	for _, fn := range guard.DangerousFunctions {
		f := strings.ToLower(strings.TrimSpace(fn))
		if f == "" {
			continue
		}
		if strings.Contains(lower, f) {
			return CheckedSQL{}, fmt.Errorf("dangerous function/expression is not allowed: %s", f)
		}
	}
	if !guard.AllowSubquery {
		if regexp.MustCompile(`(?is)\b(from|join)\s*\(`).MatchString(clean) {
			return CheckedSQL{}, fmt.Errorf("subquery in FROM/JOIN is not allowed for this datasource")
		}
	}
	tables, err := ExtractTables(clean)
	if err != nil {
		return CheckedSQL{}, err
	}
	if len(tables) == 0 {
		return CheckedSQL{}, fmt.Errorf("no table found")
	}
	if guard.RequireSchemaQualifiedTables {
		for _, t := range tables {
			if t.Schema == "" {
				return CheckedSQL{}, fmt.Errorf("table must be schema-qualified: %s", t.Table)
			}
		}
	}
	if !guard.AllowCrossSchemaJoin {
		seen := map[string]bool{}
		for _, t := range tables {
			seen[strings.ToLower(t.Schema)] = true
		}
		if len(seen) > 1 {
			return CheckedSQL{}, fmt.Errorf("cross-schema join is not allowed for this datasource")
		}
	}
	if err := enforceTablePolicies(tables, clean, guard, cat); err != nil {
		return CheckedSQL{}, err
	}
	if !limitRe.MatchString(clean) {
		if guard.RequireLimit && !guard.AppendLimitIfMissing {
			return CheckedSQL{}, fmt.Errorf("LIMIT is required")
		}
		if guard.AppendLimitIfMissing {
			clean = strings.TrimRight(clean, " ;\t\n\r") + fmt.Sprintf(" LIMIT %d", defaultLimit)
		}
	}
	if maxRows > 0 {
		clean = capLimit(clean, maxRows)
	}
	return CheckedSQL{SQL: clean, Tables: tables}, nil
}

func ExtractTables(sqlText string) ([]TableRef, error) {
	matches := tableRe.FindAllStringSubmatch(sqlText, -1)
	seen := map[string]bool{}
	out := make([]TableRef, 0, len(matches))
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		if raw == "" || strings.HasPrefix(raw, "(") {
			continue
		}
		parts := splitIdentifier(raw)
		var schemaName, tableName string
		if len(parts) == 1 {
			tableName = unquoteIdent(parts[0])
		} else if len(parts) >= 2 {
			schemaName = unquoteIdent(parts[0])
			tableName = unquoteIdent(parts[1])
		}
		if tableName == "" {
			continue
		}
		full := strings.ToLower(schemaName + "." + tableName)
		if schemaName == "" {
			full = strings.ToLower(tableName)
		}
		if !seen[full] {
			seen[full] = true
			out = append(out, TableRef{Schema: schemaName, Table: tableName, Full: full})
		}
	}
	return out, nil
}

func splitIdentifier(s string) []string {
	var parts []string
	var b strings.Builder
	inBacktick := false
	inQuote := false
	for _, r := range s {
		switch r {
		case '`':
			inBacktick = !inBacktick
			b.WriteRune(r)
		case '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case '.':
			if !inBacktick && !inQuote {
				parts = append(parts, strings.TrimSpace(b.String()))
				b.Reset()
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return parts
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '`' && s[len(s)-1] == '`') || (s[0] == '"' && s[len(s)-1] == '"')) {
		return s[1 : len(s)-1]
	}
	return s
}

func enforceTablePolicies(tables []TableRef, sqlText string, guard config.GuardConfig, cat *schema.Catalog) error {
	allowedSchemas := toSet(guard.AllowedSchemas)
	deniedSchemas := toSet(guard.DeniedSchemas)
	allowedTables := toSet(guard.AllowedTables)
	deniedTables := toSet(guard.DeniedTables)
	largeTables := toSet(guard.LargeTables)
	requireWhere := toSet(guard.RequireWhereForTables)
	requireTime := toSet(guard.RequireTimeForTables)
	for _, t := range tables {
		full := strings.ToLower(t.Schema + "." + t.Table)
		if t.Schema != "" {
			if len(allowedSchemas) > 0 && !allowedSchemas[strings.ToLower(t.Schema)] {
				return fmt.Errorf("schema is not allowed: %s", t.Schema)
			}
			if deniedSchemas[strings.ToLower(t.Schema)] {
				return fmt.Errorf("schema is denied: %s", t.Schema)
			}
		}
		if len(allowedTables) > 0 && !allowedTables[full] {
			return fmt.Errorf("table is not in allowed_tables: %s", full)
		}
		if deniedTables[full] {
			return fmt.Errorf("table is denied: %s", full)
		}
		if guard.EnforceKnownTables && t.Schema != "" && !cat.HasTable(t.Schema, t.Table) {
			return fmt.Errorf("unknown table in schema catalog: %s", full)
		}
		if guard.DenyRawDetailTablesByDefault && isRawDetailName(full) {
			return fmt.Errorf("raw detail table is denied by default: %s", full)
		}
		if largeTables[full] || requireWhere[full] {
			if !whereRe.MatchString(sqlText) {
				return fmt.Errorf("large table requires WHERE filter: %s", full)
			}
		}
		if requireTime[full] || largeTables[full] {
			if !hasAnyWord(sqlText, guard.TimeColumnHints) {
				return fmt.Errorf("large table requires time filter column: %s", full)
			}
		}
	}
	return nil
}

func isRawDetailName(full string) bool {
	parts := []string{"record", "log", "detail", "transaction", "trans", "history"}
	for _, p := range parts {
		if strings.Contains(full, p) {
			return true
		}
	}
	return false
}

func stripComments(s string) string {
	var out strings.Builder
	inSingle, inDouble, inBacktick := false, false, false
	for i := 0; i < len(s); i++ {
		if !inSingle && !inDouble && !inBacktick && i+1 < len(s) && s[i] == '-' && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			out.WriteByte('\n')
			continue
		}
		if !inSingle && !inDouble && !inBacktick && i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++
			continue
		}
		ch := s[i]
		if ch == '\'' && !inDouble && !inBacktick {
			inSingle = !inSingle
		}
		if ch == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
		}
		if ch == '`' && !inSingle && !inDouble {
			inBacktick = !inBacktick
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func hasMultipleStatements(s string) bool {
	inSingle, inDouble, inBacktick := false, false, false
	for i, r := range s {
		if r == '\'' && !inDouble && !inBacktick {
			inSingle = !inSingle
		}
		if r == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
		}
		if r == '`' && !inSingle && !inDouble {
			inBacktick = !inBacktick
		}
		if r == ';' && !inSingle && !inDouble && !inBacktick {
			rest := strings.TrimSpace(s[i+1:])
			if rest != "" {
				return true
			}
		}
	}
	return false
}

func capLimit(s string, maxRows int) string {
	return regexp.MustCompile(`(?i)\blimit\s+(\d+)`).ReplaceAllStringFunc(s, func(m string) string {
		fields := strings.Fields(m)
		if len(fields) != 2 {
			return m
		}
		var n int
		_, _ = fmt.Sscanf(fields[1], "%d", &n)
		if n <= 0 || n > maxRows {
			return "LIMIT " + fmt.Sprint(maxRows)
		}
		return m
	})
}

func toSet(items []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range items {
		v = strings.ToLower(strings.Trim(v, " `\""))
		if v != "" {
			m[v] = true
		}
	}
	return m
}

func hasAnyWord(s string, words []string) bool {
	lower := strings.ToLower(s)
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		if containsIdentifier(lower, w) {
			return true
		}
	}
	return false
}

func containsIdentifier(s, needle string) bool {
	idx := strings.Index(s, strings.ToLower(needle))
	for idx >= 0 {
		beforeOK := idx == 0 || !isIdentRune(rune(s[idx-1]))
		after := idx + len(needle)
		afterOK := after >= len(s) || !isIdentRune(rune(s[after]))
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(s[idx+1:], needle)
		if next < 0 {
			return false
		}
		idx += 1 + next
	}
	return false
}

func isIdentRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func TablesDescription(tables []TableRef) string {
	items := make([]string, 0, len(tables))
	for _, t := range tables {
		items = append(items, t.Full)
	}
	sort.Strings(items)
	return strings.Join(items, ", ")
}
