package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"log/slog"

	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

const copilotAPIURL = "https://api.githubcopilot.com/chat/completions"

// copilotMessage is an OpenAI-compatible chat message.
type copilotMessage struct {
	Role       string            `json:"role"`
	Content    interface{}       `json:"content"` // string or []copilotContentPart
	ToolCallID string            `json:"tool_call_id,omitempty"`
	ToolCalls  []copilotToolCall `json:"tool_calls,omitempty"`
}

type copilotContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type copilotToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function copilotFunctionCall `json:"function"`
}

type copilotFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type copilotResponse struct {
	Choices []struct {
		Message      copilotMessage `json:"message"`
		FinishReason string         `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// getCopilotToken retrieves the GitHub Copilot bearer token via gh CLI.
func getCopilotToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// copilotTools defines the tools available to Copilot agents.
var copilotTools = []map[string]interface{}{
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "read_file",
			"description": "Read the contents of a file",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "File path to read"},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "write_file",
			"description": "Write content to a file",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":    map[string]interface{}{"type": "string", "description": "File path to write"},
					"content": map[string]interface{}{"type": "string", "description": "Content to write"},
				},
				"required": []string{"path", "content"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "bash",
			"description": "Run a bash command and return its output",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string", "description": "Bash command to execute"},
				},
				"required": []string{"command"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "list_dir",
			"description": "List files in a directory",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "Directory path to list"},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "supermemory_store",
			"description": "Store a learning, finding, or fact in Supermemory for recall in future sessions. Call this at the end of a run to persist key insights.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{"type": "string", "description": "The content to store (finding, learning, decision, technical fact)"},
					"tags":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Tags to categorize this memory (e.g. repo name, domain, agent slug)"},
				},
				"required": []string{"content"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "supermemory_recall",
			"description": "Search Supermemory for relevant past learnings and context. Call this at the start of a run to recall domain knowledge from prior sessions.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "Search query to find relevant memories"},
					"limit": map[string]interface{}{"type": "integer", "description": "Maximum results to return (default: 5)"},
				},
				"required": []string{"query"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "wiki_search",
			"description": "Full-text search across all wiki pages (titles, slugs, and content). Returns ranked results with snippets. Use this to find relevant knowledge before starting work.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "Search terms (e.g. 'deployment runbook', 'auth architecture')"},
					"limit": map[string]interface{}{"type": "integer", "description": "Maximum results to return (default: 10)"},
				},
				"required": []string{"query"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "wiki_read",
			"description": "Read a wiki page by its slug. Returns the full page content.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"slug": map[string]interface{}{"type": "string", "description": "Page slug (e.g. 'deployment-guide', 'api-key-rotation')"},
				},
				"required": []string{"slug"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "wiki_list",
			"description": "List all wiki pages (titles and slugs). Use this to see what knowledge exists before creating new pages.",
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "wiki_create",
			"description": "Create a new wiki page. Use this to document decisions, runbooks, architecture, onboarding guides, or any durable knowledge the team should remember across runs.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title":   map[string]interface{}{"type": "string", "description": "Page title (slug auto-generated from title)"},
					"content": map[string]interface{}{"type": "string", "description": "Page content (markdown)"},
				},
				"required": []string{"title", "content"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "wiki_update",
			"description": "Update an existing wiki page. Use this to keep wiki pages current when you discover new information, complete work that changes a documented process, or fix outdated content.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"slug":    map[string]interface{}{"type": "string", "description": "Page slug to update"},
					"title":   map[string]interface{}{"type": "string", "description": "New title (optional, omit to keep current)"},
					"content": map[string]interface{}{"type": "string", "description": "New content (optional, omit to keep current)"},
				},
				"required": []string{"slug"},
			},
		},
	},
}

// executeTool runs a tool call and returns the result string.
// agentID and runID are used to log supermemory events; database may be nil (e.g. in tests).
func executeTool(name, argsJSON, workingDir, agentID, runID string, database *db.DB) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing args: %v", err)
	}

	strArg := func(key string) string {
		if v, ok := args[key]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}

	switch name {
	case "read_file":
		path := strArg("path")
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("error reading file: %v", err)
		}
		return string(data)

	case "write_file":
		path := strArg("path")
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Sprintf("error creating dirs: %v", err)
		}
		if err := os.WriteFile(path, []byte(strArg("content")), 0644); err != nil {
			return fmt.Sprintf("error writing file: %v", err)
		}
		return "file written successfully"

	case "bash":
		cmd := exec.Command("bash", "-c", strArg("command"))
		cmd.Dir = workingDir
		out, err := cmd.CombinedOutput()
		result := string(out)
		if err != nil {
			result += fmt.Sprintf("\n[exit error: %v]", err)
		}
		if len(result) > 8000 {
			result = result[:8000] + "\n... (truncated)"
		}
		return result

	case "list_dir":
		path := strArg("path")
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Sprintf("error listing dir: %v", err)
		}
		var lines []string
		for _, e := range entries {
			prefix := "  "
			if e.IsDir() {
				prefix = "d "
			}
			lines = append(lines, prefix+e.Name())
		}
		return strings.Join(lines, "\n")

	case "supermemory_store":
		apiKey := os.Getenv("SUPERMEMORY_API_KEY")
		if apiKey == "" {
			return "error: SUPERMEMORY_API_KEY not set"
		}
		content := strArg("content")
		if content == "" {
			return "error: content is required"
		}
		var tags []string
		if tagsRaw, ok := args["tags"]; ok {
			if tagsSlice, ok := tagsRaw.([]interface{}); ok {
				for _, t := range tagsSlice {
					tags = append(tags, fmt.Sprintf("%v", t))
				}
			}
		}
		tags = append(tags, "secondorder")
		payload := map[string]interface{}{"content": content, "tags": tags}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "https://api.supermemory.ai/v3/documents", bytes.NewReader(body))
		if err != nil {
			return fmt.Sprintf("error creating request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("error calling supermemory: %v", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			result := fmt.Sprintf("supermemory error %d: %s", resp.StatusCode, string(respBody))
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "store", "", 1, false)
			}
			return result
		}
		var resultMap map[string]interface{}
		var result string
		if err := json.Unmarshal(respBody, &resultMap); err == nil {
			if id, ok := resultMap["id"].(string); ok {
				result = fmt.Sprintf("stored successfully (id: %s)", id)
			}
		}
		if result == "" {
			result = "stored successfully"
		}
		if database != nil {
			database.LogSupermemoryEvent(agentID, runID, "store", "", 1, true)
		}
		return result

	case "supermemory_recall":
		apiKey := os.Getenv("SUPERMEMORY_API_KEY")
		if apiKey == "" {
			return "error: SUPERMEMORY_API_KEY not set"
		}
		query := strArg("query")
		if query == "" {
			return "error: query is required"
		}
		limit := 5
		if lv, ok := args["limit"]; ok {
			if lf, ok := lv.(float64); ok && lf > 0 {
				limit = int(lf)
			}
		}
		payload := map[string]interface{}{"q": query, "limit": limit}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "https://api.supermemory.ai/v3/search", bytes.NewReader(body))
		if err != nil {
			return fmt.Sprintf("error creating request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("error calling supermemory: %v", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			result := fmt.Sprintf("supermemory error %d: %s", resp.StatusCode, string(respBody))
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "recall", query, 0, false)
			}
			return result
		}
		var searchResp struct {
			Results []struct {
				Title  string  `json:"title"`
				Score  float64 `json:"score"`
				Chunks []struct {
					Content string `json:"content"`
				} `json:"chunks"`
			} `json:"results"`
		}
		if err := json.Unmarshal(respBody, &searchResp); err != nil {
			result := fmt.Sprintf("error parsing supermemory response: %v", err)
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "recall", query, 0, false)
			}
			return result
		}
		if len(searchResp.Results) == 0 {
			result := "no memories found for query: " + query
			if database != nil {
				database.LogSupermemoryEvent(agentID, runID, "recall", query, 0, true)
			}
			return result
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(searchResp.Results)))
		for i, r := range searchResp.Results {
			sb.WriteString(fmt.Sprintf("### %d. %s (score: %.2f)\n", i+1, r.Title, r.Score))
			for _, chunk := range r.Chunks {
				sb.WriteString(chunk.Content)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
		result := sb.String()
		resultCount := parseRecallCount(result)
		success := !strings.HasPrefix(result, "error") && !strings.HasPrefix(result, "supermemory error")
		if database != nil {
			database.LogSupermemoryEvent(agentID, runID, "recall", query, resultCount, success)
		}
		return result

	case "wiki_search":
		if database == nil {
			return "error: wiki not available (no database)"
		}
		query := strArg("query")
		if query == "" {
			return "error: query is required"
		}
		limit := 10
		if lv, ok := args["limit"]; ok {
			if lf, ok := lv.(float64); ok && lf > 0 {
				limit = int(lf)
			}
		}
		// Sanitize for FTS5: quote each token, add prefix wildcard
		tokens := strings.Fields(query)
		for i, tok := range tokens {
			tok = strings.ReplaceAll(tok, `"`, `""`)
			tokens[i] = `"` + tok + `"` + "*"
		}
		ftsQuery := strings.Join(tokens, " ")
		results, err := database.SearchWikiPagesFTS(ftsQuery, limit)
		if err != nil {
			return fmt.Sprintf("error searching wiki: %v", err)
		}
		if len(results) == 0 {
			return "no wiki pages found for: " + query
		}
		out, _ := json.Marshal(results)
		return string(out)

	case "wiki_read":
		if database == nil {
			return "error: wiki not available (no database)"
		}
		slug := strArg("slug")
		if slug == "" {
			return "error: slug is required"
		}
		page, err := database.GetWikiPageBySlug(slug)
		if err != nil {
			return fmt.Sprintf("error: wiki page %q not found", slug)
		}
		out, _ := json.Marshal(page)
		return string(out)

	case "wiki_list":
		if database == nil {
			return "error: wiki not available (no database)"
		}
		summaries, err := database.ListWikiPageSummaries()
		if err != nil {
			return fmt.Sprintf("error listing wiki: %v", err)
		}
		if len(summaries) == 0 {
			return "no wiki pages exist yet"
		}
		out, _ := json.Marshal(summaries)
		return string(out)

	case "wiki_create":
		if database == nil {
			return "error: wiki not available (no database)"
		}
		title := strArg("title")
		if title == "" {
			return "error: title is required"
		}
		slug := copilotWikiSlug(title)
		if slug == "" {
			return "error: title must contain letters or numbers"
		}
		page := &models.WikiPage{
			Slug:             slug,
			Title:            title,
			Content:          strArg("content"),
			CreatedByAgentID: &agentID,
			UpdatedByAgentID: &agentID,
		}
		if err := database.CreateWikiPage(page); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return fmt.Sprintf("error: wiki page with slug %q already exists", slug)
			}
			return fmt.Sprintf("error creating wiki page: %v", err)
		}
		return fmt.Sprintf("created wiki page %q (slug: %s)", title, slug)

	case "wiki_update":
		if database == nil {
			return "error: wiki not available (no database)"
		}
		slug := strArg("slug")
		if slug == "" {
			return "error: slug is required"
		}
		page, err := database.GetWikiPageBySlug(slug)
		if err != nil {
			return fmt.Sprintf("error: wiki page %q not found", slug)
		}
		if t := strArg("title"); t != "" {
			page.Title = t
		}
		if c := strArg("content"); c != "" {
			page.Content = c
		}
		page.UpdatedByAgentID = &agentID
		if err := database.UpdateWikiPage(page); err != nil {
			return fmt.Sprintf("error updating wiki page: %v", err)
		}
		return fmt.Sprintf("updated wiki page %q", slug)

	default:
		return fmt.Sprintf("unknown tool: %s", name)
	}
}

var copilotWikiSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func copilotWikiSlug(title string) string {
	v := strings.ToLower(strings.TrimSpace(title))
	v = copilotWikiSlugRe.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	return v
}

// parseRecallCount extracts N from "Found N memories:\n\n..."
// Returns 0 if result starts with "no memories found" or on parse failure.
func parseRecallCount(result string) int {
	if strings.HasPrefix(result, "no memories found") {
		return 0
	}
	var n int
	fmt.Sscanf(result, "Found %d memories", &n)
	return n
}

func (s *Scheduler) execCopilot(ctx context.Context, agent *models.Agent, soAPIKey, runID, issueKey, prompt string) (string, error) {
	token, err := getCopilotToken()
	if err != nil {
		return "", fmt.Errorf("copilot auth: %w", err)
	}

	// Build system prompt: archetype + artifact-docs/CLAUDE.md
	var systemParts []string

	if agent.ArchetypeSlug != "" {
		content, err := archetypes.Get(agent.ArchetypeSlug)
		if err == nil && content != "" {
			systemParts = append(systemParts, content)
		}
	}

	// Load artifact-docs/CLAUDE.md if present
	claudeMD := filepath.Join(agent.WorkingDir, "artifact-docs", "CLAUDE.md")
	if data, err := os.ReadFile(claudeMD); err == nil {
		systemParts = append(systemParts, "## Project Context\n\n"+string(data))
	}

	// Load artifact-docs/hot-memory.md if present
	hotMem := filepath.Join(agent.WorkingDir, "artifact-docs", "hot-memory.md")
	if data, err := os.ReadFile(hotMem); err == nil {
		systemParts = append(systemParts, "## Working Memory\n\n"+string(data))
	}

	// Inject secondorder API connection info
	systemParts = append(systemParts, fmt.Sprintf(`## secondorder API

You have access to the secondorder task board via REST API at %s.
Your API key: %s
Your agent ID: %s
Current issue key: %s
Working directory: %s

Use the bash tool to call the API with curl when you need to update issue status, post comments, or create sub-issues.`, fmt.Sprintf("http://localhost:%d", s.port), soAPIKey, agent.ID, issueKey, agent.WorkingDir))

	systemPrompt := strings.Join(systemParts, "\n\n---\n\n")

	messages := []copilotMessage{}
	if systemPrompt != "" {
		messages = append(messages, copilotMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, copilotMessage{Role: "user", Content: prompt})

	model := agent.Model
	if model == "" {
		model = "claude-sonnet-4.6"
	}

	var outputBuf strings.Builder
	maxTurns := agent.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	for turn := 0; turn < maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return outputBuf.String(), ctx.Err()
		default:
		}

		reqBody := map[string]interface{}{
			"model":    model,
			"messages": messages,
			"tools":    copilotTools,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return outputBuf.String(), fmt.Errorf("marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", copilotAPIURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return outputBuf.String(), fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Copilot-Integration-Id", "vscode-chat")
		req.Header.Set("Editor-Version", "vscode/1.99.0")

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return outputBuf.String(), fmt.Errorf("API request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return outputBuf.String(), fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode != 200 {
			return outputBuf.String(), fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp copilotResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			return outputBuf.String(), fmt.Errorf("parse response: %w", err)
		}

		if len(apiResp.Choices) == 0 {
			break
		}

		// The Copilot API may return multiple choices: one with text content and
		// one with tool_calls. Merge them into a single synthetic assistant message.
		var merged copilotMessage
		merged.Role = "assistant"
		for _, c := range apiResp.Choices {
			if s, ok := c.Message.Content.(string); ok && s != "" && merged.Content == nil {
				merged.Content = s
			}
			if len(c.Message.ToolCalls) > 0 {
				merged.ToolCalls = append(merged.ToolCalls, c.Message.ToolCalls...)
			}
		}
		msg := merged
		messages = append(messages, msg)

		// Collect text output
		if s, ok := msg.Content.(string); ok && s != "" {
			outputBuf.WriteString(s)
			outputBuf.WriteString("\n")
		}

		// Handle tool calls
		if len(msg.ToolCalls) == 0 {
			slog.Debug("copilot: agent done (no tool calls)", "turn", turn, "run_id", runID)
			break
		}

		for _, tc := range msg.ToolCalls {
			slog.Debug("copilot: tool call", "tool", tc.Function.Name, "run_id", runID)
			result := executeTool(tc.Function.Name, tc.Function.Arguments, agent.WorkingDir, agent.ID, runID, s.db)
			outputBuf.WriteString(fmt.Sprintf("[tool:%s] %s\n", tc.Function.Name, result))
			messages = append(messages, copilotMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return outputBuf.String(), nil
}
