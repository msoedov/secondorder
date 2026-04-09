package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// prURLPattern matches GitHub PR URLs.
// Example: https://github.com/owner/repo/pull/123
var prURLPattern = regexp.MustCompile(`https?://github\.com/([^/\s]+)/([^/\s]+)/pull/(\d+)`)

// PRInfo holds parsed info from a GitHub PR URL.
type PRInfo struct {
	URL        string
	Owner      string
	Repo       string
	Number     int
	ReviewData *PRReviewData // nil if fetch failed / rate-limited
	FetchError string
}

// PRReviewData holds fetched review state for a single PR.
type PRReviewData struct {
	State            string // "open", "approved", "changes_requested", "mixed"
	ApprovalCount    int
	TotalReviews     int
	HasBotBlock      bool   // CodeRabbit or Gemini has changes_requested
	BotBlockers      []string
	FetchedAt        time.Time
}

// gitHubToken returns the GitHub token from env, or "" if none.
func gitHubToken() string {
	for _, envVar := range []string{"GITHUB_PAT", "GITHUB_TOKEN", "GH_TOKEN"} {
		if t := os.Getenv(envVar); t != "" {
			return t
		}
	}
	return ""
}

// ExtractPRURLs scans comment bodies for GitHub PR URLs and returns unique PRInfo slices.
func ExtractPRURLs(commentBodies []string) []PRInfo {
	seen := make(map[string]bool)
	var prs []PRInfo
	for _, body := range commentBodies {
		matches := prURLPattern.FindAllStringSubmatch(body, -1)
		for _, m := range matches {
			url := m[0]
			if seen[url] {
				continue
			}
			seen[url] = true
			num, _ := strconv.Atoi(m[3])
			prs = append(prs, PRInfo{
				URL:    url,
				Owner:  m[1],
				Repo:   m[2],
				Number: num,
			})
		}
	}
	return prs
}

// botReviewerNames are the known CI/review bots we flag as blockers.
var botReviewerNames = []string{
	"coderabbitai",
	"gemini-code-assist",
	"github-actions",
	"copilot",
}

func isBotReviewer(login string) bool {
	lower := strings.ToLower(login)
	for _, b := range botReviewerNames {
		if strings.Contains(lower, b) {
			return true
		}
	}
	return false
}

// FetchPRReviews calls the GitHub REST API for reviews on a PR.
// On rate-limit or auth failure it returns a PRReviewData with FetchError set.
func FetchPRReviews(pr PRInfo) PRInfo {
	token := gitHubToken()

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/reviews",
		pr.Owner, pr.Repo, pr.Number)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		pr.FetchError = fmt.Sprintf("request build error: %v", err)
		return pr
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		pr.FetchError = fmt.Sprintf("fetch error: %v", err)
		return pr
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		pr.FetchError = "GitHub API rate-limited or access denied"
		return pr
	}
	if resp.StatusCode == http.StatusNotFound {
		pr.FetchError = "PR not found or repository is private"
		return pr
	}
	if resp.StatusCode != http.StatusOK {
		pr.FetchError = fmt.Sprintf("GitHub API returned HTTP %d", resp.StatusCode)
		return pr
	}

	var reviews []struct {
		State               string `json:"state"`
		User                struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
		SubmittedAt time.Time `json:"submitted_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		pr.FetchError = fmt.Sprintf("decode error: %v", err)
		return pr
	}

	// Summarise reviews — only the latest review per user counts.
	latestByUser := make(map[string]string)
	for _, r := range reviews {
		login := r.User.Login
		state := strings.ToUpper(r.State)
		// COMMENTED, DISMISSED states don't override an existing APPROVED/CHANGES_REQUESTED
		if state == "COMMENTED" || state == "DISMISSED" {
			if _, exists := latestByUser[login]; !exists {
				latestByUser[login] = state
			}
			continue
		}
		latestByUser[login] = state
	}

	data := &PRReviewData{
		FetchedAt: time.Now(),
	}

	approvals := 0
	changesRequested := false
	var botBlockers []string

	for login, state := range latestByUser {
		data.TotalReviews++
		if state == "APPROVED" {
			approvals++
		}
		if state == "CHANGES_REQUESTED" {
			changesRequested = true
			if isBotReviewer(login) {
				botBlockers = append(botBlockers, login)
			}
		}
	}

	data.ApprovalCount = approvals
	data.BotBlockers = botBlockers
	data.HasBotBlock = len(botBlockers) > 0

	// Determine overall state
	if len(latestByUser) == 0 {
		data.State = "open"
	} else if changesRequested {
		data.State = "changes_requested"
	} else if approvals > 0 {
		data.State = "approved"
	} else {
		data.State = "open"
	}

	pr.ReviewData = data
	return pr
}

// FetchAllPRReviews fetches review data for each PR concurrently (max 5).
func FetchAllPRReviews(prs []PRInfo) []PRInfo {
	if len(prs) == 0 {
		return prs
	}

	type result struct {
		idx int
		pr  PRInfo
	}
	ch := make(chan result, len(prs))
	sem := make(chan struct{}, 5) // concurrency limit

	for i, pr := range prs {
		go func(idx int, p PRInfo) {
			sem <- struct{}{}
			defer func() { <-sem }()
			ch <- result{idx: idx, pr: FetchPRReviews(p)}
		}(i, pr)
	}

	out := make([]PRInfo, len(prs))
	for range prs {
		r := <-ch
		out[r.idx] = r.pr
	}
	return out
}
