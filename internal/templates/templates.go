package templates

import (
	"embed"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"
)

//go:embed *.html
var fs embed.FS

func Parse() (*template.Template, error) {
	root := template.New("").Funcs(funcMap)

	partialFiles := []string{"partials.html"}
	pageFiles := []string{
		"dashboard.html",
		"issues.html",
		"issue_detail.html",
		"agents.html",
		"agent_detail.html",
		"run_detail.html",
		"work_blocks.html",
		"work_block_detail.html",
	}

	for _, pf := range partialFiles {
		data, err := fs.ReadFile(pf)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", pf, err)
		}
		if _, err := root.Parse(string(data)); err != nil {
			return nil, fmt.Errorf("parse %s: %w", pf, err)
		}
	}

	for _, pf := range pageFiles {
		data, err := fs.ReadFile(pf)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", pf, err)
		}
		name := strings.TrimSuffix(pf, ".html")
		if _, err := root.New(name).Parse(string(data)); err != nil {
			return nil, fmt.Errorf("parse %s: %w", pf, err)
		}
	}

	return root, nil
}

var funcMap = template.FuncMap{
	"timeAgo":        timeAgo,
	"statusColor":    statusColor,
	"statusIcon":     statusIcon,
	"statusDot":      statusDot,
	"priorityLabel":  priorityLabel,
	"priorityColor":  priorityColor,
	"runStatusColor": runStatusColor,
	"formatCost":     formatCost,
	"formatTokens":   formatTokens,
	"truncate":       truncate,
	"deref":          deref,
	"diffLines":      diffLines,
	"nl2br":          nl2br,
	"upper":          strings.ToUpper,
	"shortID": func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	},
	"seq": seq,
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"wbStatusColor": wbStatusColor,
	"derefTime": func(t *time.Time) time.Time {
		if t == nil {
			return time.Time{}
		}
		return *t
	},
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

func statusColor(s string) string {
	switch s {
	case "todo":
		return "badge-todo"
	case "in_progress":
		return "badge-in-progress"
	case "in_review":
		return "badge-in-review"
	case "done":
		return "badge-done"
	case "blocked":
		return "badge-blocked"
	case "cancelled":
		return "badge-cancelled"
	default:
		return "badge-default"
	}
}

func statusIcon(s string) template.HTML {
	switch s {
	case "todo":
		return `<svg class="icon-xs" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/></svg>`
	case "in_progress":
		return `<svg class="icon-xs" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><path d="M8 2a6 6 0 0 0 0 12" fill="currentColor" opacity=".35"/></svg>`
	case "in_review":
		return `<svg class="icon-xs" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><circle cx="8" cy="8" r="2.5" fill="currentColor" opacity=".4"/></svg>`
	case "done":
		return `<svg class="icon-xs" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><path d="M5.5 8l2 2 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	case "blocked":
		return `<svg class="icon-xs" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><path d="M5.5 5.5l5 5M10.5 5.5l-5 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`
	case "cancelled":
		return `<svg class="icon-xs" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5" stroke-dasharray="3 2"/></svg>`
	default:
		return ""
	}
}

func statusDot(s string) string {
	switch s {
	case "todo":
		return "dot-muted"
	case "in_progress":
		return "dot-blue"
	case "in_review":
		return "dot-amber"
	case "done":
		return "dot-green"
	case "blocked":
		return "dot-red"
	case "cancelled":
		return "dot-dim"
	default:
		return "dot-muted"
	}
}

func priorityLabel(p int) string {
	switch p {
	case 1:
		return "Low"
	case 2:
		return "Medium"
	case 3:
		return "High"
	case 4:
		return "Urgent"
	default:
		return ""
	}
}

func priorityColor(p int) string {
	switch p {
	case 1:
		return "priority-low"
	case 2:
		return "priority-medium"
	case 3:
		return "priority-high"
	case 4:
		return "priority-urgent"
	default:
		return "priority-none"
	}
}

func runStatusColor(s string) string {
	switch s {
	case "running":
		return "badge-in-progress"
	case "completed":
		return "badge-done"
	case "failed":
		return "badge-blocked"
	case "cancelled":
		return "badge-cancelled"
	default:
		return "badge-default"
	}
}

func formatCost(f float64) string {
	if f == 0 {
		return "$0.00"
	}
	if f < 0.01 {
		return fmt.Sprintf("$%.4f", f)
	}
	return fmt.Sprintf("$%.2f", f)
}

func formatTokens(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type DiffLine struct {
	Class   string
	Content string
}

func diffLines(diff string) []DiffLine {
	if diff == "" {
		return nil
	}
	var lines []DiffLine
	for _, line := range strings.Split(diff, "\n") {
		dl := DiffLine{Content: line}
		switch {
		case strings.HasPrefix(line, "@@"):
			dl.Class = "diff-hunk"
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "diff "):
			dl.Class = "diff-meta"
		case strings.HasPrefix(line, "+"):
			dl.Class = "diff-add"
		case strings.HasPrefix(line, "-"):
			dl.Class = "diff-del"
		default:
			dl.Class = "diff-context"
		}
		lines = append(lines, dl)
	}
	return lines
}

func nl2br(s string) template.HTML {
	return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
}

func wbStatusColor(s string) string {
	switch s {
	case "proposed":
		return "badge-proposed"
	case "active":
		return "badge-in-progress"
	case "ready":
		return "badge-in-review"
	case "shipped":
		return "badge-done"
	case "cancelled":
		return "badge-cancelled"
	default:
		return "badge-default"
	}
}

func seq(n int) []int {
	s := make([]int, int(math.Max(0, float64(n))))
	for i := range s {
		s[i] = i
	}
	return s
}
