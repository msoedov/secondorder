package templates

import (
	"html/template"
	"testing"
)

func TestLinkTickets(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want template.HTML
	}{
		{"plain text", "no tickets here", "no tickets here"},
		{"single ticket", "see SO-5 for details", `see <a href="/issues/SO-5" class="text-ac-t hover:underline">SO-5</a> for details`},
		{"multiple tickets", "SO-1 and SO-42", `<a href="/issues/SO-1" class="text-ac-t hover:underline">SO-1</a> and <a href="/issues/SO-42" class="text-ac-t hover:underline">SO-42</a>`},
		{"with newlines", "line1\nSO-3", `line1<br><a href="/issues/SO-3" class="text-ac-t hover:underline">SO-3</a>`},
		{"html escaped", "SO-1 <script>", `<a href="/issues/SO-1" class="text-ac-t hover:underline">SO-1</a> &lt;script&gt;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkTickets(tt.in)
			if got != tt.want {
				t.Errorf("linkTickets(%q)\n got: %s\nwant: %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractProject(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"fix(bootc-ecosystem): repair something", "bootc-ecosystem"},
		{"feat(cncf-darkmode): add dark mode toggle", "cncf-darkmode"},
		{"feat(dashboard): add per-project filter labels to issues dashboard", "dashboard"},
		{"fix(bluefin-lts): align CI", "bluefin-lts"},
		{"fix(powerlevel): pin ci.yml Go version", "powerlevel"},
		{"feat(cncf-darkmode/ci): update workflow", "cncf-darkmode"},
		{"fix(ui): activity feed timeAgo timestamps freeze", "ui"},
		{"fix(scheduler): auto-cleanup stale runs", "scheduler"},
		// bare prefix (no verb)
		{"mesa: update config", "mesa"},
		// no scope → empty
		{"add something without scope", ""},
		{"SO-55: some issue", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := extractProject(tt.title)
			if got != tt.want {
				t.Errorf("extractProject(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}
