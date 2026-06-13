package formatter

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"nl2sql-executor-go-prod/internal/dbresult"
)

type TextBundle struct {
	Summary    string
	AnswerText string
	TableText  string
}

func BuildText(title string, result *dbresult.Result, maxInlineRows int) TextBundle {
	if maxInlineRows <= 0 {
		maxInlineRows = 30
	}
	summary := fmt.Sprintf("查询：%s\n数据源：%s\n节点：%s\n返回行数：%d，耗时：%d ms", title, result.Datasource, result.Host, result.RowCount, result.DurationMS)
	if result.Truncated {
		summary += "\n注意：结果已按最大行数截断。"
	}
	answer := NaturalLanguageSummary(title, result)
	return TextBundle{Summary: summary, AnswerText: answer, TableText: MarkdownTable(result, maxInlineRows)}
}

// NaturalLanguageSummary turns small aggregate query results into a concise business answer.
// It is intentionally heuristic: when a query returns KPI-style columns, Telegram users see
// sentences like "昨日美国VPBET：总成功充值金额：100万；总提现金额：50万" instead of a raw table.
func NaturalLanguageSummary(title string, result *dbresult.Result) string {
	if result == nil || len(result.Columns) == 0 || len(result.Rows) == 0 || len(result.Rows) > 20 {
		return ""
	}
	colKinds := make([]string, len(result.Columns))
	metricCount := 0
	for i, c := range result.Columns {
		kind := metricKind(c)
		colKinds[i] = kind
		if kind != "" {
			metricCount++
		}
	}
	if metricCount == 0 {
		return ""
	}

	var lines []string
	for _, row := range result.Rows {
		labels := rowLabels(result.Columns, colKinds, row)
		prefix := strings.Join(labels, " ")
		if prefix == "" && len(result.Rows) == 1 {
			prefix = conciseTitle(title)
		}
		var metrics []string
		for i, kind := range colKinds {
			if kind == "" || i >= len(row) {
				continue
			}
			value := formatMetricValue(row[i], kind)
			metrics = append(metrics, fmt.Sprintf("%s：%s", metricLabel(kind), value))
		}
		if len(metrics) == 0 {
			continue
		}
		if prefix == "" {
			lines = append(lines, strings.Join(metrics, "；"))
		} else {
			lines = append(lines, fmt.Sprintf("%s：%s", prefix, strings.Join(metrics, "；")))
		}
	}
	return strings.Join(lines, "\n")
}

func rowLabels(cols []string, kinds []string, row []string) []string {
	var out []string
	for i, col := range cols {
		if i >= len(row) || kinds[i] != "" {
			continue
		}
		v := strings.TrimSpace(row[i])
		if v == "" || strings.EqualFold(v, "NULL") {
			continue
		}
		lc := strings.ToLower(strings.TrimSpace(col))
		if looksNumeric(v) && !isLikelyLabelColumn(lc) {
			continue
		}
		if strings.Contains(lc, "date") || lc == "day" || lc == "dt" || strings.Contains(lc, "time") {
			continue
		}
		out = append(out, v)
	}
	return out
}

func conciseTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "查询结果"
	}
	for _, p := range []string{"搜索", "查询", "统计", "查"} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "查询结果"
	}
	return s
}

func metricKind(column string) string {
	c := normalizeName(column)
	switch {
	case containsAll(c, "recharge", "submit") || strings.Contains(c, "submit_recharge") || strings.Contains(c, "total_submit"):
		return "submit_recharge_amount"
	case containsAll(c, "recharge", "success") || strings.Contains(c, "base_recharge_amount_total") || strings.Contains(c, "recharge_amount_total"):
		return "success_recharge_amount"
	case containsAll(c, "withdraw", "submit") || containsAll(c, "exchange", "submit"):
		return "submit_withdraw_amount"
	case containsAll(c, "withdraw", "success") || strings.Contains(c, "base_withdraw_money_amount_total") || strings.Contains(c, "withdraw_money_total") || strings.Contains(c, "withdraw_amount"):
		return "success_withdraw_amount"
	case strings.Contains(c, "success_amount") || strings.Contains(c, "success_base_money"):
		return "success_amount"
	case strings.Contains(c, "submit_amount") || strings.Contains(c, "submit_base_money"):
		return "submit_amount"
	case strings.Contains(c, "recharge_count"):
		return "recharge_count"
	case strings.Contains(c, "withdraw_count") || strings.Contains(c, "exchange_count"):
		return "withdraw_count"
	case strings.Contains(c, "user_count") || strings.HasSuffix(c, "_users"):
		return "user_count"
	default:
		return ""
	}
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, " `\"")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func metricLabel(kind string) string {
	switch kind {
	case "success_recharge_amount":
		return "总成功充值金额"
	case "submit_recharge_amount":
		return "总提交充值金额"
	case "success_withdraw_amount":
		return "总提现金额"
	case "submit_withdraw_amount":
		return "总提交提现金额"
	case "success_amount":
		return "总成功金额"
	case "submit_amount":
		return "总提交金额"
	case "recharge_count":
		return "充值次数"
	case "withdraw_count":
		return "提现次数"
	case "user_count":
		return "用户数"
	default:
		return "数值"
	}
}

func formatMetricValue(s string, kind string) string {
	f, ok := ToFloat(s)
	if !ok {
		return strings.TrimSpace(s)
	}
	if strings.Contains(kind, "count") || kind == "user_count" {
		return formatInteger(f)
	}
	return formatChineseAmount(f)
}

func formatInteger(f float64) string {
	return strconv.FormatInt(int64(math.Round(f)), 10)
}

func formatChineseAmount(f float64) string {
	abs := math.Abs(f)
	sign := ""
	if f < 0 {
		sign = "-"
	}
	switch {
	case abs >= 100000000:
		return sign + trimZeros(abs/100000000) + "亿"
	case abs >= 10000:
		return sign + trimZeros(abs/10000) + "万"
	default:
		return sign + trimZeros(abs)
	}
}

func trimZeros(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", f), "0"), ".")
}

func looksNumeric(s string) bool {
	_, ok := ToFloat(s)
	return ok
}

func isLikelyLabelColumn(c string) bool {
	return c == "tid" || strings.Contains(c, "id") || strings.Contains(c, "code") || strings.Contains(c, "area") || strings.Contains(c, "country") || strings.Contains(c, "platform") || strings.Contains(c, "channel") || strings.Contains(c, "agent") || strings.Contains(c, "target") || strings.Contains(c, "name")
}

func MarkdownTable(result *dbresult.Result, maxRows int) string {
	if result == nil || len(result.Columns) == 0 {
		return ""
	}
	rows := result.Rows
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	widths := make([]int, len(result.Columns))
	for i, c := range result.Columns {
		widths[i] = min(max(runeLen(c), 3), 30)
	}
	for _, r := range rows {
		for i, v := range r {
			if i < len(widths) {
				widths[i] = min(max(widths[i], runeLen(v)), 30)
			}
		}
	}
	var b strings.Builder
	b.WriteString("\n```\n")
	writeRow(&b, result.Columns, widths)
	sep := make([]string, len(widths))
	for i, w := range widths {
		sep[i] = strings.Repeat("-", w)
	}
	writeRow(&b, sep, widths)
	for _, r := range rows {
		writeRow(&b, r, widths)
	}
	if result.RowCount > len(rows) {
		b.WriteString(fmt.Sprintf("... only first %d rows displayed, full result in file\n", len(rows)))
	}
	b.WriteString("```")
	return b.String()
}

func writeRow(b *strings.Builder, cells []string, widths []int) {
	for i, w := range widths {
		v := ""
		if i < len(cells) {
			v = truncate(cells[i], w)
		}
		b.WriteString(padRight(v, w))
		if i < len(widths)-1 {
			b.WriteString(" | ")
		}
	}
	b.WriteByte('\n')
}

func WriteCSV(dir, jobID string, result *dbresult.Result, compressThreshold int) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeName(jobID)+".csv")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	w := csv.NewWriter(f)
	_ = w.Write(result.Columns)
	for _, r := range result.Rows {
		_ = w.Write(r)
	}
	w.Flush()
	cerr := f.Close()
	if err := w.Error(); err != nil {
		return "", err
	}
	if cerr != nil {
		return "", cerr
	}
	info, err := os.Stat(path)
	if err == nil && compressThreshold > 0 && info.Size() > int64(compressThreshold) {
		gzPath, err := gzipFile(path)
		if err != nil {
			return path, nil
		}
		_ = os.Remove(path)
		return gzPath, nil
	}
	return path, nil
}

func gzipFile(path string) (string, error) {
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()
	outPath := path + ".gz"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(out)
	_, err = in.WriteTo(gz)
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return outPath, err
}

func SplitText(s string, chunkSize int) []string {
	if chunkSize <= 0 {
		chunkSize = 3500
	}
	if utf8.RuneCountInString(s) <= chunkSize {
		return []string{s}
	}
	runes := []rune(s)
	var parts []string
	for len(runes) > 0 {
		n := chunkSize
		if len(runes) < n {
			n = len(runes)
		}
		cut := n
		for i := n - 1; i > 0 && i > n-200; i-- {
			if runes[i] == '\n' {
				cut = i + 1
				break
			}
		}
		parts = append(parts, string(runes[:cut]))
		runes = runes[cut:]
	}
	for i := range parts {
		parts[i] = fmt.Sprintf("[%d/%d]\n%s", i+1, len(parts), parts[i])
	}
	return parts
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "result"
	}
	return b.String()
}

func truncate(s string, n int) string {
	if runeLen(s) <= n {
		return s
	}
	r := []rune(s)
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
func padRight(s string, n int) string { return s + strings.Repeat(" ", max(0, n-runeLen(s))) }
func runeLen(s string) int            { return utf8.RuneCountInString(s) }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ToFloat(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" || strings.EqualFold(s, "NULL") {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}
