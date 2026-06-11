package chart

import (
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"strings"

	"nl2sql-executor-go-prod/internal/dbresult"
	"nl2sql-executor-go-prod/internal/formatter"
)

type Hint struct {
	Type   string   `json:"type"`
	X      string   `json:"x"`
	Y      []string `json:"y"`
	Series string   `json:"series"`
	Reason string   `json:"reason"`
}

func WriteSVG(dir, jobID, title string, result *dbresult.Result, hint Hint) (string, bool, error) {
	if result == nil || len(result.Rows) == 0 || len(hint.Y) == 0 || hint.X == "" {
		return "", false, nil
	}
	xi := colIndex(result.Columns, hint.X)
	yi := colIndex(result.Columns, hint.Y[0])
	if xi < 0 || yi < 0 {
		return "", false, nil
	}
	type pt struct {
		x string
		y float64
	}
	pts := make([]pt, 0, len(result.Rows))
	for _, r := range result.Rows {
		if xi >= len(r) || yi >= len(r) {
			continue
		}
		v, ok := formatter.ToFloat(r[yi])
		if !ok {
			continue
		}
		pts = append(pts, pt{x: r[xi], y: v})
		if len(pts) >= 50 {
			break
		}
	}
	if len(pts) == 0 {
		return "", false, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", false, err
	}
	path := filepath.Join(dir, safeName(jobID)+".svg")
	maxY := pts[0].y
	minY := pts[0].y
	for _, p := range pts {
		if p.y > maxY {
			maxY = p.y
		}
		if p.y < minY {
			minY = p.y
		}
	}
	if maxY == minY {
		maxY += 1
		minY = 0
	}
	w, h := 900.0, 520.0
	left, right, top, bottom := 80.0, 30.0, 70.0, 90.0
	plotW, plotH := w-left-right, h-top-bottom
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, w, h, w, h))
	b.WriteString(`<rect width="100%" height="100%" fill="white"/>`)
	b.WriteString(fmt.Sprintf(`<text x="%.0f" y="35" font-size="22" font-family="Arial" font-weight="600">%s</text>`, left, html.EscapeString(title)))
	b.WriteString(fmt.Sprintf(`<text x="%.0f" y="58" font-size="13" font-family="Arial" fill="#555">%s by %s</text>`, left, html.EscapeString(hint.Y[0]), html.EscapeString(hint.X)))
	b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#333"/>`, left, top+plotH, left+plotW, top+plotH))
	b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#333"/>`, left, top, left, top+plotH))
	chartType := strings.ToLower(hint.Type)
	if chartType == "line" && len(pts) > 1 {
		var coords []string
		for i, p := range pts {
			coords = append(coords, fmt.Sprintf("%.1f,%.1f", xPos(i, len(pts), left, plotW), yPos(p.y, minY, maxY, top, plotH)))
		}
		b.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#2563eb" stroke-width="3"/>`, strings.Join(coords, " ")))
		for i, p := range pts {
			x := xPos(i, len(pts), left, plotW)
			y := yPos(p.y, minY, maxY, top, plotH)
			b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="3" fill="#2563eb"/>`, x, y))
		}
	} else {
		barGap := 4.0
		barW := math.Max(4, plotW/float64(len(pts))-barGap)
		for i, p := range pts {
			x := left + float64(i)*(plotW/float64(len(pts))) + barGap/2
			y := yPos(p.y, minY, maxY, top, plotH)
			bh := top + plotH - y
			b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="2" fill="#2563eb"/>`, x, y, barW, bh))
		}
	}
	for i, p := range pts {
		if i%max(1, len(pts)/10) == 0 || len(pts) <= 12 {
			x := xPos(i, len(pts), left, plotW)
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" font-size="11" font-family="Arial" text-anchor="end" transform="rotate(-35 %.1f %.1f)">%s</text>`, x, top+plotH+22, x, top+plotH+22, html.EscapeString(trunc(p.x, 18))))
		}
	}
	for i := 0; i <= 5; i++ {
		y := top + plotH - float64(i)*plotH/5
		val := minY + (maxY-minY)*float64(i)/5
		b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#e5e7eb"/>`, left, y, left+plotW, y))
		b.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.1f" font-size="11" font-family="Arial" text-anchor="end">%.2f</text>`, left-8, y+4, val))
	}
	b.WriteString(`</svg>`)
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func colIndex(cols []string, name string) int {
	for i, c := range cols {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return -1
}
func xPos(i, n int, left, plotW float64) float64 {
	if n <= 1 {
		return left + plotW/2
	}
	return left + float64(i)*plotW/float64(n-1)
}
func yPos(v, minY, maxY, top, plotH float64) float64 { return top + plotH - (v-minY)/(maxY-minY)*plotH }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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
		return "chart"
	}
	return b.String()
}
