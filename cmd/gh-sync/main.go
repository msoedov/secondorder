// cmd/gh-sync/main.go — sync castrojo/* GitHub issues labeled copilot-ready into secondorder DB
//
// Usage:
//   gh-sync [-db PATH] [-repo OWNER/REPO] [-dry-run]
//
// Reads copilot-ready issues via gh CLI, inserts new ones into secondorder SQLite DB.
// Auto-assigns to CEO. Idempotent: skips issues that already exist by title.
// Run via systemd timer or manually.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"database/sql"
)

// ghIssue is the subset of GitHub issue fields we care about.
type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// routeSlug picks a secondorder agent slug from issue labels + title keywords.
func routeSlug(issue ghIssue) string {
	labelSet := map[string]bool{}
	for _, l := range issue.Labels {
		labelSet[strings.ToLower(l.Name)] = true
	}
	title := strings.ToLower(issue.Title)

	switch {
	case labelSet["domain:bluefin"] || labelSet["domain:lts"] || labelSet["domain:aurora"] ||
		strings.Contains(title, "bluefin") || strings.Contains(title, "aurora") || strings.Contains(title, "lts"):
		return "bluefin-engineer"
	case labelSet["domain:cncf"] || labelSet["domain:website"] ||
		strings.Contains(title, "cncf") || strings.Contains(title, "locale") || strings.Contains(title, "i18n"):
		return "cncf-engineer"
	case labelSet["kind:quality"] || strings.Contains(title, "test") || strings.Contains(title, "qa"):
		return "qa-engineer"
	case labelSet["kind:docs"] || strings.Contains(title, "doc") || strings.Contains(title, "readme"):
		return "docs-writer"
	default:
		return "ceo"
	}
}

func main() {
	dbPath := flag.String("db", os.ExpandEnv("$HOME/.local/share/secondorder/so.db"), "path to secondorder SQLite DB")
	reposFlag := flag.String("repos", "castrojo/copilot-config", "comma-separated list of OWNER/REPO to sync")
	dryRun := flag.Bool("dry-run", false, "print actions without writing to DB")
	flag.Parse()

	repos := strings.Split(*reposFlag, ",")

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Load agent slug → ID mapping from DB
	agentIDs := map[string]string{}
	rows, err := db.Query(`SELECT slug, id FROM agents`)
	if err != nil {
		slog.Error("query agents", "err", err)
		os.Exit(1)
	}
	for rows.Next() {
		var slug, id string
		_ = rows.Scan(&slug, &id)
		agentIDs[slug] = id
	}
	rows.Close()

	ceoID := agentIDs["ceo"]
	if ceoID == "" {
		slog.Error("CEO agent not found in DB — is secondorder bootstrapped?")
		os.Exit(1)
	}

	// Fetch issue key prefix from settings
	issuePrefix := "SO"
	_ = db.QueryRow(`SELECT value FROM settings WHERE key='issue_prefix'`).Scan(&issuePrefix)

	synced, skipped, errors := 0, 0, 0

	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		slog.Info("fetching issues", "repo", repo)

		out, err := exec.Command("gh", "issue", "list",
			"--repo", repo,
			"--label", "copilot-ready",
			"--state", "open",
			"--json", "number,title,body,url,labels",
			"--limit", "50",
		).Output()
		if err != nil {
			slog.Error("gh issue list", "repo", repo, "err", err)
			errors++
			continue
		}

		var issues []ghIssue
		if err := json.Unmarshal(out, &issues); err != nil {
			slog.Error("parse gh output", "err", err)
			errors++
			continue
		}

		for _, gh := range issues {
			title := fmt.Sprintf("[GH %s#%d] %s", repo, gh.Number, gh.Title)

			// Idempotency: skip if title already exists
			var existingKey string
			err := db.QueryRow(`SELECT key FROM issues WHERE title = ?`, title).Scan(&existingKey)
			if err == nil {
				slog.Info("skipping — already synced", "key", existingKey, "title", title)
				skipped++
				continue
			}

			// Determine assignee
			assigneeSlug := routeSlug(gh)
			assigneeID := agentIDs[assigneeSlug]
			if assigneeID == "" {
				assigneeID = ceoID
				assigneeSlug = "ceo"
			}

			// Generate next issue key
			var maxKey string
			_ = db.QueryRow(`SELECT COALESCE(MAX(key),'') FROM issues`).Scan(&maxKey)
			nextNum := 1
			if maxKey != "" {
				fmt.Sscanf(strings.TrimPrefix(maxKey, issuePrefix+"-"), "%d", &nextNum)
				nextNum++
			}
			key := fmt.Sprintf("%s-%d", issuePrefix, nextNum)

			description := gh.Body
			if description == "" {
				description = fmt.Sprintf("Source: %s", gh.URL)
			} else {
				description = fmt.Sprintf("%s\n\n---\nSource: %s", description, gh.URL)
			}

			if *dryRun {
				slog.Info("dry-run: would insert",
					"key", key,
					"title", title,
					"assignee", assigneeSlug,
				)
				synced++
				continue
			}

			now := time.Now().UTC().Format(time.RFC3339)
			_, err = db.Exec(`
				INSERT INTO issues (id, key, title, description, status, priority, assignee_agent_id, created_at, updated_at, type, stages, current_stage_id)
				VALUES (?, ?, ?, ?, 'todo', 0, ?, ?, ?, 'task', '[]', 0)`,
				uuid.New().String(), key, title, description, assigneeID, now, now,
			)
			if err != nil {
				slog.Error("insert issue", "title", title, "err", err)
				errors++
				continue
			}

			slog.Info("synced",
				"key", key,
				"title", title,
				"assignee", assigneeSlug,
				"gh_url", gh.URL,
			)
			synced++
		}
	}

	slog.Info("done", "synced", synced, "skipped", skipped, "errors", errors)
	if errors > 0 {
		os.Exit(1)
	}
}
