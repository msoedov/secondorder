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
	"strings"
	"time"

	"log/slog"

	"github.com/msoedov/secondorder/internal/archetypes"
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
}

// executeTool runs a tool call and returns the result string.
func executeTool(name, argsJSON, workingDir string) string {
	var args map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error parsing args: %v", err)
	}

	switch name {
	case "read_file":
		path := args["path"]
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("error reading file: %v", err)
		}
		return string(data)

	case "write_file":
		path := args["path"]
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Sprintf("error creating dirs: %v", err)
		}
		if err := os.WriteFile(path, []byte(args["content"]), 0644); err != nil {
			return fmt.Sprintf("error writing file: %v", err)
		}
		return "file written successfully"

	case "bash":
		cmd := exec.Command("bash", "-c", args["command"])
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
		path := args["path"]
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

	default:
		return fmt.Sprintf("unknown tool: %s", name)
	}
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
			result := executeTool(tc.Function.Name, tc.Function.Arguments, agent.WorkingDir)
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
