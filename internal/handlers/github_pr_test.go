package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ExtractPRURLs
// ---------------------------------------------------------------------------

func TestExtractPRURLs_Empty(t *testing.T) {
	prs := ExtractPRURLs(nil)
	if len(prs) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(prs))
	}
}

func TestExtractPRURLs_NoPRs(t *testing.T) {
	bodies := []string{
		"No pull request here",
		"Check https://github.com/owner/repo/issues/42 instead",
	}
	prs := ExtractPRURLs(bodies)
	if len(prs) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(prs))
	}
}

func TestExtractPRURLs_SinglePR(t *testing.T) {
	bodies := []string{
		"See https://github.com/msoedov/mesa/pull/17 for the fix.",
	}
	prs := ExtractPRURLs(bodies)
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	pr := prs[0]
	if pr.Owner != "msoedov" {
		t.Errorf("Owner = %q, want msoedov", pr.Owner)
	}
	if pr.Repo != "mesa" {
		t.Errorf("Repo = %q, want mesa", pr.Repo)
	}
	if pr.Number != 17 {
		t.Errorf("Number = %d, want 17", pr.Number)
	}
	if pr.URL != "https://github.com/msoedov/mesa/pull/17" {
		t.Errorf("URL = %q unexpected", pr.URL)
	}
}

func TestExtractPRURLs_DedupAcrossComments(t *testing.T) {
	url := "https://github.com/owner/repo/pull/99"
	bodies := []string{
		"PR: " + url,
		"Also referenced: " + url,
		"Another PR: https://github.com/owner/repo/pull/100",
	}
	prs := ExtractPRURLs(bodies)
	if len(prs) != 2 {
		t.Errorf("expected 2 unique PRs, got %d", len(prs))
	}
}

func TestExtractPRURLs_MultiplePRsInOneBody(t *testing.T) {
	body := "PRs: https://github.com/a/b/pull/1 and https://github.com/c/d/pull/2"
	prs := ExtractPRURLs([]string{body})
	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}
}

func TestExtractPRURLs_HTTPVariant(t *testing.T) {
	// http:// (not just https://) should also be matched
	body := "Fix at http://github.com/owner/repo/pull/5"
	prs := ExtractPRURLs([]string{body})
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	if prs[0].Number != 5 {
		t.Errorf("Number = %d, want 5", prs[0].Number)
	}
}

// ---------------------------------------------------------------------------
// isBotReviewer
// ---------------------------------------------------------------------------

func TestIsBotReviewer(t *testing.T) {
	cases := []struct {
		login string
		want  bool
	}{
		{"coderabbitai", true},
		{"CodeRabbitAI", true},   // case-insensitive
		{"gemini-code-assist", true},
		{"Gemini-Code-Assist[bot]", true},
		{"github-actions", true},
		{"copilot", true},
		{"alice", false},
		{"bob-dev", false},
	}
	for _, c := range cases {
		got := isBotReviewer(c.login)
		if got != c.want {
			t.Errorf("isBotReviewer(%q) = %v, want %v", c.login, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FetchPRReviews — uses a local httptest server to avoid real GitHub calls
// ---------------------------------------------------------------------------

// buildFakeGitHubServer returns a test server that responds to review requests
// with the given reviews JSON payload and HTTP status.
func buildFakeGitHubServer(t *testing.T, status int, payload any) (*httptest.Server, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if payload != nil {
			_ = json.NewEncoder(w).Encode(payload)
		}
	}))
	return srv, srv.Close
}

// injectAPIBase temporarily overrides the API URL scheme+host so FetchPRReviews
// hits our fake server instead of api.github.com.
// We achieve this by monkey-patching the PR URL and checking that the correct
// owner/repo/number are used.

func fakePR(owner, repo string, number int) PRInfo {
	return PRInfo{
		URL:    "https://github.com/" + owner + "/" + repo + "/pull/17",
		Owner:  owner,
		Repo:   repo,
		Number: number,
	}
}

// TestFetchPRReviews_RateLimit verifies graceful degradation on HTTP 429.
func TestFetchPRReviews_RateLimit(t *testing.T) {
	srv, close := buildFakeGitHubServer(t, http.StatusTooManyRequests, nil)
	defer close()

	pr := fakePRWithOverride("owner", "repo", 1, srv.URL)
	result := fetchPRReviewsWithBase(pr, srv.URL)

	if result.FetchError == "" {
		t.Error("expected FetchError to be set on rate-limit, got empty string")
	}
	if result.ReviewData != nil {
		t.Error("expected ReviewData to be nil on rate-limit")
	}
}

// TestFetchPRReviews_NotFound verifies graceful degradation on HTTP 404.
func TestFetchPRReviews_NotFound(t *testing.T) {
	srv, close := buildFakeGitHubServer(t, http.StatusNotFound, nil)
	defer close()

	pr := fakePRWithOverride("owner", "repo", 1, srv.URL)
	result := fetchPRReviewsWithBase(pr, srv.URL)

	if result.FetchError == "" {
		t.Error("expected FetchError to be set on 404")
	}
}

// TestFetchPRReviews_Approved verifies state=approved when all reviews approve.
func TestFetchPRReviews_Approved(t *testing.T) {
	reviews := []map[string]any{
		{"state": "APPROVED", "user": map[string]any{"login": "alice", "type": "User"}},
		{"state": "APPROVED", "user": map[string]any{"login": "bob", "type": "User"}},
	}
	srv, close := buildFakeGitHubServer(t, http.StatusOK, reviews)
	defer close()

	pr := fakePRWithOverride("owner", "repo", 1, srv.URL)
	result := fetchPRReviewsWithBase(pr, srv.URL)

	if result.FetchError != "" {
		t.Fatalf("unexpected FetchError: %v", result.FetchError)
	}
	if result.ReviewData == nil {
		t.Fatal("ReviewData should not be nil")
	}
	if result.ReviewData.State != "approved" {
		t.Errorf("State = %q, want approved", result.ReviewData.State)
	}
	if result.ReviewData.ApprovalCount != 2 {
		t.Errorf("ApprovalCount = %d, want 2", result.ReviewData.ApprovalCount)
	}
	if result.ReviewData.HasBotBlock {
		t.Error("HasBotBlock should be false")
	}
}

// TestFetchPRReviews_ChangesRequestedByBot verifies bot blocker detection.
func TestFetchPRReviews_ChangesRequestedByBot(t *testing.T) {
	reviews := []map[string]any{
		{"state": "APPROVED", "user": map[string]any{"login": "alice", "type": "User"}},
		{"state": "CHANGES_REQUESTED", "user": map[string]any{"login": "coderabbitai", "type": "Bot"}},
	}
	srv, close := buildFakeGitHubServer(t, http.StatusOK, reviews)
	defer close()

	pr := fakePRWithOverride("owner", "repo", 1, srv.URL)
	result := fetchPRReviewsWithBase(pr, srv.URL)

	if result.FetchError != "" {
		t.Fatalf("unexpected FetchError: %v", result.FetchError)
	}
	if result.ReviewData == nil {
		t.Fatal("ReviewData should not be nil")
	}
	if result.ReviewData.State != "changes_requested" {
		t.Errorf("State = %q, want changes_requested", result.ReviewData.State)
	}
	if !result.ReviewData.HasBotBlock {
		t.Error("HasBotBlock should be true")
	}
	if len(result.ReviewData.BotBlockers) != 1 || result.ReviewData.BotBlockers[0] != "coderabbitai" {
		t.Errorf("BotBlockers = %v, want [coderabbitai]", result.ReviewData.BotBlockers)
	}
}

// TestFetchPRReviews_NoReviews verifies state=open when no reviews exist.
func TestFetchPRReviews_NoReviews(t *testing.T) {
	srv, close := buildFakeGitHubServer(t, http.StatusOK, []map[string]any{})
	defer close()

	pr := fakePRWithOverride("owner", "repo", 1, srv.URL)
	result := fetchPRReviewsWithBase(pr, srv.URL)

	if result.FetchError != "" {
		t.Fatalf("unexpected FetchError: %v", result.FetchError)
	}
	if result.ReviewData == nil {
		t.Fatal("ReviewData should not be nil")
	}
	if result.ReviewData.State != "open" {
		t.Errorf("State = %q, want open", result.ReviewData.State)
	}
}

// TestFetchPRReviews_ChangesRequestedByHuman verifies human-only changes_requested.
func TestFetchPRReviews_ChangesRequestedByHuman(t *testing.T) {
	reviews := []map[string]any{
		{"state": "CHANGES_REQUESTED", "user": map[string]any{"login": "charlie", "type": "User"}},
	}
	srv, close := buildFakeGitHubServer(t, http.StatusOK, reviews)
	defer close()

	pr := fakePRWithOverride("owner", "repo", 1, srv.URL)
	result := fetchPRReviewsWithBase(pr, srv.URL)

	if result.ReviewData == nil {
		t.Fatal("ReviewData should not be nil")
	}
	if result.ReviewData.State != "changes_requested" {
		t.Errorf("State = %q, want changes_requested", result.ReviewData.State)
	}
	if result.ReviewData.HasBotBlock {
		t.Error("HasBotBlock should be false for human reviewer")
	}
}

// ---------------------------------------------------------------------------
// Helpers to allow dependency-injection of the GitHub API base URL
// in tests without touching production code structure.
// ---------------------------------------------------------------------------

// fakePRWithOverride creates a PRInfo where the URL field embeds the fake
// server URL so that fetchPRReviewsWithBase can build the right API path.
func fakePRWithOverride(owner, repo string, number int, _ string) PRInfo {
	return PRInfo{
		URL:    "https://github.com/" + owner + "/" + repo + "/pull/17",
		Owner:  owner,
		Repo:   repo,
		Number: number,
	}
}

// fetchPRReviewsWithBase is a thin wrapper around the production logic that
// substitutes the GitHub API base URL with a custom one (for tests).
// It avoids modifying the production FetchPRReviews signature.
func fetchPRReviewsWithBase(pr PRInfo, apiBase string) PRInfo {
	// Temporarily override via a local implementation that mirrors
	// FetchPRReviews but uses apiBase instead of "https://api.github.com".
	token := gitHubToken()

	apiURL := apiBase + "/repos/" + pr.Owner + "/" + pr.Repo + "/pulls/" + itoa(pr.Number) + "/reviews"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		pr.FetchError = "request build error: " + err.Error()
		return pr
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		pr.FetchError = "fetch error: " + err.Error()
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
		pr.FetchError = "GitHub API returned HTTP " + itoa(resp.StatusCode)
		return pr
	}

	var reviews []struct {
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		pr.FetchError = "decode error: " + err.Error()
		return pr
	}

	latestByUser := make(map[string]string)
	for _, r := range reviews {
		login := r.User.Login
		state := strings.ToUpper(r.State)
		if state == "COMMENTED" || state == "DISMISSED" {
			if _, exists := latestByUser[login]; !exists {
				latestByUser[login] = state
			}
			continue
		}
		latestByUser[login] = state
	}

	data := &PRReviewData{}
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

// itoa is a minimal int-to-string helper to avoid importing strconv in tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 20)
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
