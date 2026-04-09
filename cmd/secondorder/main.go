package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"log/slog"

	"github.com/lmittmann/tint"
	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/discord"
	"github.com/msoedov/secondorder/internal/handlers"
	"github.com/msoedov/secondorder/internal/models"
	"github.com/msoedov/secondorder/internal/scheduler"
	"github.com/msoedov/secondorder/internal/telegram"
	"github.com/msoedov/secondorder/internal/templates"
	"github.com/msoedov/secondorder/static"
	"golang.org/x/term"
)

//go:embed templates/*.json
var startupTemplatesFS embed.FS

func main() {
	// slog level set after CLI parsing; initialize with default
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelWarn)
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		TimeFormat: time.TimeOnly,
		Level:      logLevel,
	})))

	port := envOr("PORT", "3001")
	dbPath := envOr("DB", "so.db")
	archetypesDir := envOr("ARCHETYPES", "archetypes")
	templateName := envOr("TEMPLATE", "startup")
	archetypes.SetOverridesDir(archetypesDir)

	defaultModel := envOr("MODEL", "claude")

	var templateProvided, modelProvided bool

	verbosity := 0

	// CLI: secondorder [-t <template>] [-m <model>] [-v|-vv|-vvv] [port]
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			fmt.Println("Usage: secondorder [-t <template>] [-m <model>] [-v|-vv|-vvv] [port]")
			fmt.Println("  -t, --template  Team template: startup, dev-team, enterprise, saas, agency (default: startup)")
			fmt.Println("  -m, --model     Default agent runner: claude, gemini, codex, opencode (default: claude)")
			fmt.Println("  -v              Verbosity: -v info, -vv debug, -vvv debug+cmd")
			fmt.Println("  port            HTTP port (default: 3001, or PORT env)")
			os.Exit(0)
		} else if arg == "-vvv" {
			verbosity = 3
		} else if arg == "-vv" {
			verbosity = 2
		} else if arg == "-v" {
			verbosity = 1
		} else if arg == "--template" || arg == "-t" {
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --template requires a value")
				os.Exit(1)
			}
			i++
			templateName = args[i]
			templateProvided = true
		} else if arg == "--model" || arg == "-m" {
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --model requires a value")
				os.Exit(1)
			}
			i++
			defaultModel = args[i]
			modelProvided = true
		} else {
			port = arg
		}
	}

	switch {
	case verbosity >= 3:
		logLevel.Set(slog.Level(-8)) // trace: per-line agent output
	case verbosity == 2:
		logLevel.Set(slog.LevelDebug)
	case verbosity == 1:
		logLevel.Set(slog.LevelInfo)
	}

	// Database
	database, err := db.Open(dbPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	// HTML templates
	tmpl, err := templates.Parse()
	if err != nil {
		slog.Error("failed to parse templates", "error", err)
		os.Exit(1)
	}

	// SSE hub
	sse := handlers.NewSSEHub()

	// Scheduler
	portInt := 3001
	if p, err := parsePort(port); err == nil {
		portInt = p
	}
	sched := scheduler.New(database, portInt)

	if modelProvided {
		runner := resolveRunner(defaultModel)
		model := "default"
		if m, ok := models.RunnerModels[runner]; ok && len(m) > 0 {
			model = m[0]
		}
		sched.SetOverrideModel(runner, model)
		slog.Warn("session override: all agents will use", "runner", runner, "model", model)
	}

	// Set activity callback
	sched.SetOnActivity(func(action, entityType, entityID string, agentID *string, details string) {
		handlers.LogActivityAndBroadcast(database, sse, tmpl, action, entityType, entityID, agentID, details)
	})

	// Wire wake function

	wake := sched.WakeAgent

	// Callbacks
	sched.SetOnRunStart(func(run *models.Run) {
		data, _ := json.Marshal(map[string]string{
			"run_id":   run.ID,
			"agent_id": run.AgentID,
			"status":   run.Status,
			"mode":     run.Mode,
		})
		sse.Broadcast("run_started", string(data))
	})

	sched.SetOnRunComplete(func(run *models.Run) {
		data, _ := json.Marshal(map[string]string{
			"run_id":   run.ID,
			"agent_id": run.AgentID,
			"status":   run.Status,
			"mode":     run.Mode,
		})
		sse.Broadcast("run_complete", string(data))

		// Update audit_runs when an audit run completes
		if run.Mode == "audit" {
			status := "completed"
			if run.Status == "failed" || run.Status == "cancelled" {
				status = run.Status
			}
			database.Exec(`UPDATE audit_runs SET status=?, completed_at=datetime('now') WHERE run_id=? AND status='running'`,
				status, run.ID)
		}
	})

	// Telegram bot (optional, requires feature flag)
	var tg handlers.TelegramNotifier
	if token := os.Getenv("TELEGRAM_TOKEN"); token != "" && database.IsFeatureEnabled("telegram") {
		chatID := os.Getenv("TELEGRAM_CHAT_ID")
		bot := telegram.New(token, chatID)
		bot.OnApproval = func(blockID, decision string) {
			if decision == "approve" {
				wb, err := database.GetWorkBlock(blockID)
				if err != nil {
					slog.Error("telegram: block not found", "error", err)
					return
				}
				switch wb.Status {
				case models.WBStatusProposed:
					database.UpdateWorkBlockStatus(blockID, models.WBStatusActive)
					slog.Info("telegram: block activated", "block", wb.Title)
					if ceo, err := database.GetCEOAgent(); err == nil {
						sched.WakeAgentHeartbeat(ceo)
					}
				case models.WBStatusReady:
					database.UpdateWorkBlockStatus(blockID, models.WBStatusShipped)
					slog.Info("telegram: block shipped", "block", wb.Title)
				}
			} else {
				handlers.LogActivityAndBroadcast(database, sse, tmpl, "rejected", "work_block", blockID, nil, "Rejected via Telegram")

				slog.Info("telegram: block rejected", "block", blockID)
			}
		}
		ctx, tgCancel := context.WithCancel(context.Background())
		go bot.StartPolling(ctx)
		defer tgCancel()
		tg = bot
		slog.Info("telegram bot enabled")
	}

	// Discord webhook (optional, requires feature flag)
	var dc handlers.DiscordNotifier
	if webhookURL, _ := database.GetSetting("discord_webhook_url"); webhookURL != "" && database.IsFeatureEnabled("discord") {
		notifier, err := discord.New(webhookURL)
		if err != nil {
			slog.Warn("discord: failed to create notifier", "error", err)
		} else {
			dc = notifier
			slog.Info("discord webhook enabled")
		}
	}

	// Handlers
	api := handlers.NewAPI(database, sse, tmpl, wake, tg, dc)
	ui := handlers.NewUI(database, sse, tmpl, wake, sched)
	// Routes
	mux := http.NewServeMux()

	// UI routes
	mux.HandleFunc("GET /dashboard", ui.Dashboard)
	mux.HandleFunc("GET /strategy", ui.StrategyPage)
	mux.HandleFunc("POST /strategy", ui.StrategyPage)
	mux.HandleFunc("POST /strategy/apex/{id}", ui.UpdateApexBlockUI)
	mux.HandleFunc("GET /issues", ui.ListIssues)
	mux.HandleFunc("POST /issues", ui.ListIssues)
	mux.HandleFunc("GET /issues/{key}", ui.IssueDetail)
	mux.HandleFunc("POST /issues/{key}", ui.IssueDetail)
	mux.HandleFunc("GET /issues/{key}/pr-status", ui.IssuePRStatus)
	mux.HandleFunc("GET /agents", ui.ListAgents)
	mux.HandleFunc("POST /agents", ui.ListAgents)
	mux.HandleFunc("GET /agents/{slug}", ui.AgentDetail)
	mux.HandleFunc("POST /agents/{slug}", ui.AgentDetail)
	mux.HandleFunc("POST /agents/{slug}/heartbeat", ui.AgentHeartbeat)
	mux.HandleFunc("POST /agents/{slug}/assign", ui.AgentAssign)
	mux.HandleFunc("POST /scheduler/pause", ui.SchedulerPause)
	mux.HandleFunc("POST /scheduler/resume", ui.SchedulerResume)
	mux.HandleFunc("GET /work-blocks", ui.ListWorkBlocks)
	mux.HandleFunc("POST /work-blocks", ui.ListWorkBlocks)
	mux.HandleFunc("GET /work-blocks/{id}", ui.WorkBlockDetail)
	mux.HandleFunc("POST /work-blocks/{id}", ui.WorkBlockDetail)
	mux.HandleFunc("GET /activity", ui.ActivityPage)
	mux.HandleFunc("GET /policies", ui.PoliciesPage)
	mux.HandleFunc("POST /policies", ui.PoliciesPage)
	mux.HandleFunc("GET /settings", ui.Settings)
	mux.HandleFunc("POST /settings", ui.Settings)
	mux.HandleFunc("GET /crons", ui.ListCrons)
	mux.HandleFunc("POST /crons", ui.ListCrons)
	mux.HandleFunc("POST /crons/{id}", ui.CronAction)
	mux.HandleFunc("GET /runs/{id}", ui.RunDetail)
	mux.HandleFunc("GET /runs/{id}/stdout", ui.RunStdout)
	mux.HandleFunc("GET /search", ui.SearchIssuesAndAgents)
	mux.HandleFunc("GET /events", sse.ServeHTTP)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))

	// Favicon handlers for various browsers/clients
	mux.HandleFunc("GET /favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(static.FS, "favicon.svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(data)
	})
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/favicon.svg", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			ui.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	})

	// API routes (auth-wrapped)
	mux.HandleFunc("GET /api/v1/inbox", api.Auth(api.Inbox))
	mux.HandleFunc("GET /api/v1/issues/{key}", api.Auth(api.GetIssue))
	mux.HandleFunc("POST /api/v1/issues/{key}/checkout", api.Auth(api.CheckoutIssue))
	mux.HandleFunc("PATCH /api/v1/issues/{key}", api.Auth(api.UpdateIssue))
	mux.HandleFunc("DELETE /api/v1/issues/{key}", api.Auth(api.DeleteIssue))
	mux.HandleFunc("POST /api/v1/issues/{key}/comments", api.Auth(api.CreateComment))
	mux.HandleFunc("POST /api/v1/issues", api.Auth(api.CreateIssue))
	mux.HandleFunc("GET /api/v1/agents", api.Auth(api.ListAgents))
	mux.HandleFunc("GET /api/v1/agents/me", api.Auth(api.AgentMe))
	mux.HandleFunc("GET /api/v1/usage", api.Auth(api.Usage))
	mux.HandleFunc("POST /api/v1/approvals/{id}/resolve", api.Auth(api.ResolveApproval))
	mux.HandleFunc("GET /api/v1/work-blocks", api.Auth(api.ListWorkBlocks))
	mux.HandleFunc("GET /api/v1/work-blocks/{id}", api.Auth(api.GetWorkBlock))
	mux.HandleFunc("POST /api/v1/work-blocks", api.Auth(api.CreateWorkBlock))
	mux.HandleFunc("PATCH /api/v1/work-blocks/{id}", api.Auth(api.UpdateWorkBlock))
	mux.HandleFunc("POST /api/v1/work-blocks/{id}/issues", api.Auth(api.AssignIssueToBlock))
	mux.HandleFunc("DELETE /api/v1/work-blocks/{id}/issues/{key}", api.Auth(api.UnassignIssueFromBlock))

	mux.HandleFunc("GET /api/v1/apex-blocks", api.Auth(api.ListApexBlocks))
	mux.HandleFunc("GET /api/v1/apex-blocks/{id}", api.Auth(api.GetApexBlock))
	mux.HandleFunc("POST /api/v1/apex-blocks", api.Auth(api.CreateApexBlock))
	mux.HandleFunc("PATCH /api/v1/apex-blocks/{id}", api.Auth(api.UpdateApexBlock))
	mux.HandleFunc("POST /api/v1/archetype-patches", api.Auth(api.CreateArchetypePatch))

	// Webhook routes (no auth required, uses HMAC signature)
	webhookAPI := handlers.NewWebhookAPI(database, sse)
	mux.HandleFunc("POST /api/v1/webhooks/{source}/issues", webhookAPI.WebhookAuth(webhookAPI.HandleIssues))
	mux.HandleFunc("POST /api/v1/webhooks/{source}/comments", webhookAPI.WebhookAuth(webhookAPI.HandleComments))

	// Apply org template on first run
	templateName, defaultModel = promptFirstRun(database, templateName, defaultModel, templateProvided, modelProvided)
	applyStartupTemplate(database, templateName, defaultModel)

	// Recover stuck issues from previous run
	if recovered := sched.RecoverStuckIssues(); recovered > 0 {
		slog.Info("startup: recovered stuck issues", "count", recovered)
	}

	// Heartbeat loop
	sched.StartHeartbeatLoop(1 * time.Hour)

	// HTTP server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // disabled for SSE
	}

	go func() {
		slog.Info("secondorder running", "url", "http://localhost:"+port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down...")

	sse.Close()
	sched.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("http shutdown error", "error", err)
	}

	database.Close()
	slog.Info("shutdown complete")
}

func resolveRunner(model string) string {
	switch model {
	case "claude":
		return "claude_code"
	case "copilot":
		return "copilot"
	case "opencode":
		return "opencode"
	case "gemini", "codex":
		return model
	default:
		return model
	}
}

func applyStartupTemplate(database *db.DB, templateName, defaultModel string) {
	if templateName == "blank" {
		slog.Info("startup: blank template selected, skipping agent seeding")
		return
	}

	agents, err := database.ListAgents()
	if err != nil {
		slog.Error("failed to check agents table", "error", err)
		return
	}
	if len(agents) > 0 {
		return
	}

	data, err := startupTemplatesFS.ReadFile("templates/" + templateName + ".json")
	if err != nil {
		slog.Warn("startup template not found, skipping", "error", err)
		return
	}

	var tmpl struct {
		Name   string `json:"name"`
		Agents []struct {
			Name             string `json:"name"`
			Slug             string `json:"slug"`
			ArchetypeSlug    string `json:"archetype_slug"`
			Model            string `json:"model"`
			Runner           string `json:"runner"`
			WorkingDir       string `json:"working_dir"`
			HeartbeatEnabled bool   `json:"heartbeat_enabled"`
			ChromeEnabled    bool   `json:"chrome_enabled"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &tmpl); err != nil {
		slog.Error("failed to parse startup template", "error", err)
		return
	}

	slog.Info("applying org template", "template", tmpl.Name, "agents", len(tmpl.Agents))

	runner := resolveRunner(defaultModel)

	for _, a := range tmpl.Agents {
		agentRunner := runner
		if a.Runner != "" {
			agentRunner = a.Runner
		}
		agentWorkingDir := "."
		if a.WorkingDir != "" {
			agentWorkingDir = a.WorkingDir
		}

		if !models.IsValidModelForRunner(agentRunner, a.Model) {
			if m, ok := models.RunnerModels[agentRunner]; ok && len(m) > 0 {
				a.Model = m[0]
			}
		}

		agent := &models.Agent{
			ID:               uuid.New().String(),
			Name:             a.Name,
			Slug:             a.Slug,
			ArchetypeSlug:    a.ArchetypeSlug,
			Model:            a.Model,
			Runner:           agentRunner,
			WorkingDir:       agentWorkingDir,
			MaxTurns:         50,
			TimeoutSec:       models.DefaultAgentTimeoutSec,
			HeartbeatEnabled: a.HeartbeatEnabled,
			ChromeEnabled:    a.ChromeEnabled,
			Active:           true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if err := database.CreateAgent(agent); err != nil {
			slog.Error("failed to create agent", "agent", a.Name, "error", err)
			continue
		}
		slog.Info("created agent", "name", a.Name, "slug", a.Slug, "archetype", a.ArchetypeSlug, "model", a.Model)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parsePort(s string) (int, error) {
	var p int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, os.ErrInvalid
		}
		p = p*10 + int(c-'0')
	}
	return p, nil
}

func promptFirstRun(database *db.DB, templateName, defaultModel string, templateProvided, modelProvided bool) (string, string) {
	agents, _ := database.ListAgents()
	if len(agents) > 0 {
		return templateName, defaultModel
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return templateName, defaultModel
	}

	reader := bufio.NewReader(os.Stdin)

	if !templateProvided {
		fmt.Println("\nSelect a team template:")
		fmt.Println("  1. startup     - Founding team: CEO, Engineer, Product, Designer, QA, DevOps")
		fmt.Println("  2. dev-team    - Engineering-focused team")
		fmt.Println("  3. saas        - SaaS product team")
		fmt.Println("  4. agency      - Agency delivery team")
		fmt.Println("  5. enterprise  - Larger org structure")
		fmt.Println("  6. blank       - No agents, configure manually")

		prompt := func() string {
			fmt.Print("\nEnter choice [1]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				return "startup"
			}
			switch input {
			case "1", "startup":
				return "startup"
			case "2", "dev-team":
				return "dev-team"
			case "3", "saas":
				return "saas"
			case "4", "agency":
				return "agency"
			case "5", "enterprise":
				return "enterprise"
			case "6", "blank":
				return "blank"
			default:
				return ""
			}
		}

		templateName = prompt()
		if templateName == "" {
			fmt.Println("Invalid choice. Please try again.")
			templateName = prompt()
			if templateName == "" {
				fmt.Println("Invalid choice. Using default: startup")
				templateName = "startup"
			}
		}
	}

	if templateName == "blank" {
		return "blank", ""
	}

	if !modelProvided {
		fmt.Println("\nSelect default agent runner:")
		fmt.Println("  1. claude    - Claude Code (default)")
		fmt.Println("  2. gemini    - Google Gemini")
		fmt.Println("  3. codex     - OpenAI Codex")
		fmt.Println("  4. opencode  - OpenCode")

		prompt := func() string {
			fmt.Print("\nEnter choice [1]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				return "claude"
			}
			switch input {
			case "1", "claude":
				return "claude"
			case "2", "gemini":
				return "gemini"
			case "3", "codex":
				return "codex"
			case "4", "opencode":
				return "opencode"
			default:
				return ""
			}
		}

		defaultModel = prompt()
		if defaultModel == "" {
			fmt.Println("Invalid choice. Please try again.")
			defaultModel = prompt()
			if defaultModel == "" {
				fmt.Println("Invalid choice. Using default: claude")
				defaultModel = "claude"
			}
		}
	}

	fmt.Printf("\nStarting with template=%s runner=%s\n\n", templateName, defaultModel)
	return templateName, defaultModel
}
