package formatter

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"nl2sql-executor-go-prod/internal/dbresult"
)

type TextBundle struct {
	Summary   string
	TableText string
}

func BuildText(title string, result *dbresult.Result, maxInlineRows int) TextBundle {
	if maxInlineRows <= 0 {
		maxInlineRows = 30
	}
	summary := fmt.Sprintf("查询：%s\n数据源：%s\n节点：%s\n返回行数：%d，耗时：%d ms", title, result.Datasource, result.Host, result.RowCount, result.DurationMS)
	if result.Truncated {
		summary += "\n注意：结果已按最大行数截断。"
	}
	return TextBundle{Summary: summary, TableText: MarkdownTable(result, maxInlineRows)}
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
