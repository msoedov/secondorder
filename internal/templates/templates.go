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
		"policies.html",
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
		return "bg-zinc-600/80 text-zinc-200"
	case "in_progress":
		return "bg-blue-500/15 text-blue-400 ring-1 ring-inset ring-blue-500/25"
	case "in_review":
		return "bg-amber-500/15 text-amber-400 ring-1 ring-inset ring-amber-500/25"
	case "done":
		return "bg-emerald-500/15 text-emerald-400 ring-1 ring-inset ring-emerald-500/25"
	case "blocked":
		return "bg-red-500/15 text-red-400 ring-1 ring-inset ring-red-500/25"
	case "cancelled":
		return "bg-zinc-500/10 text-zinc-500 ring-1 ring-inset ring-zinc-500/20"
	default:
		return "bg-zinc-700 text-zinc-300"
	}
}

func statusIcon(s string) template.HTML {
	switch s {
	case "todo":
		return `<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/></svg>`
	case "in_progress":
		return `<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><path d="M8 2a6 6 0 0 0 0 12" fill="currentColor" opacity=".35"/></svg>`
	case "in_review":
		return `<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><circle cx="8" cy="8" r="2.5" fill="currentColor" opacity=".4"/></svg>`
	case "done":
		return `<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><path d="M5.5 8l2 2 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	case "blocked":
		return `<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/><path d="M5.5 5.5l5 5M10.5 5.5l-5 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`
	case "cancelled":
		return `<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5" stroke-dasharray="3 2"/></svg>`
	default:
		return ""
	}
}

func statusDot(s string) string {
	switch s {
	case "todo":
		return "text-zinc-400"
	case "in_progress":
		return "text-blue-400"
	case "in_review":
		return "text-amber-400"
	case "done":
		return "text-emerald-400"
	case "blocked":
		return "text-red-400"
	case "cancelled":
		return "text-zinc-600"
	default:
		return "text-zinc-400"
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
		return "text-emerald-400"
	case 2:
		return "text-yellow-400"
	case 3:
		return "text-orange-400"
	case 4:
		return "text-red-400"
	default:
		return "text-zinc-500"
	}
}

func runStatusColor(s string) string {
	switch s {
	case "running":
		return "bg-blue-500/15 text-blue-400 ring-1 ring-inset ring-blue-500/25"
	case "completed":
		return "bg-emerald-500/15 text-emerald-400 ring-1 ring-inset ring-emerald-500/25"
	case "failed":
		return "bg-red-500/15 text-red-400 ring-1 ring-inset ring-red-500/25"
	case "cancelled":
		return "bg-zinc-500/10 text-zinc-500 ring-1 ring-inset ring-zinc-500/20"
	default:
		return "bg-zinc-700 text-zinc-300"
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
			dl.Class = "text-indigo-400 bg-indigo-950/20"
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "diff "):
			dl.Class = "text-zinc-500 font-medium"
		case strings.HasPrefix(line, "+"):
			dl.Class = "text-emerald-400 bg-emerald-950/30"
		case strings.HasPrefix(line, "-"):
			dl.Class = "text-red-400 bg-red-950/30"
		default:
			dl.Class = "text-zinc-500"
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
		return "bg-purple-500/15 text-purple-400 ring-1 ring-inset ring-purple-500/25"
	case "active":
		return "bg-blue-500/15 text-blue-400 ring-1 ring-inset ring-blue-500/25"
	case "ready":
		return "bg-amber-500/15 text-amber-400 ring-1 ring-inset ring-amber-500/25"
	case "shipped":
		return "bg-emerald-500/15 text-emerald-400 ring-1 ring-inset ring-emerald-500/25"
	case "cancelled":
		return "bg-zinc-500/10 text-zinc-500 ring-1 ring-inset ring-zinc-500/20"
	default:
		return "bg-zinc-700 text-zinc-300"
	}
}

func seq(n int) []int {
	s := make([]int, int(math.Max(0, float64(n))))
	for i := range s {
		s[i] = i
	}
	return s
}
