package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/msoedov/mesa/internal/models"
)

const githubCommitsAPI = "https://api.github.com/repos/msoedov/mesa/commits/main"

type versionCheckResult struct {
	CurrentDate string `json:"current_date"`
	LatestDate  string `json:"latest_date"`
	LatestSHA   string `json:"latest_sha"`
	UpToDate    bool   `json:"up_to_date"`
	Error       string `json:"error,omitempty"`
}

func (u *UI) CheckForUpdates(w http.ResponseWriter, r *http.Request) {
	result := versionCheckResult{
		CurrentDate: models.BuildDate,
	}

	req, err := http.NewRequest("GET", githubCommitsAPI, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to build request: %v", err)
		jsonOK(w, result)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if tok := gitHubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("network error: %v", err)
		jsonOK(w, result)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		result.Error = "GitHub API rate limit exceeded — try again later"
		jsonOK(w, result)
		return
	}
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("GitHub API returned HTTP %d", resp.StatusCode)
		jsonOK(w, result)
		return
	}

	var commit struct {
		SHA    string `json:"sha"`
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commit); err != nil {
		result.Error = fmt.Sprintf("failed to parse GitHub response: %v", err)
		jsonOK(w, result)
		return
	}

	result.LatestDate = commit.Commit.Committer.Date.UTC().Format(time.RFC3339)
	result.LatestSHA = commit.SHA[:7]

	if models.BuildDate == "unknown" || models.BuildDate == "" {
		result.UpToDate = false
	} else {
		current, err := time.Parse(time.RFC3339, models.BuildDate)
		if err != nil {
			result.UpToDate = false
		} else {
			result.UpToDate = !current.Before(commit.Commit.Committer.Date.UTC())
		}
	}

	jsonOK(w, result)
}
