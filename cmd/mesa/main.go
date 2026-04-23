package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"log/slog"

	"github.com/google/uuid"

	"github.com/lmittmann/tint"
	"github.com/msoedov/mesa/internal/archetypes"
	"github.com/msoedov/mesa/internal/db"
	"github.com/msoedov/mesa/internal/discord"
	"github.com/msoedov/mesa/internal/handlers"
	"github.com/msoedov/mesa/internal/models"
	"github.com/msoedov/mesa/internal/scheduler"
	"github.com/msoedov/mesa/internal/telegram"
	"github.com/msoedov/mesa/internal/templates"
	"github.com/msoedov/mesa/static"
	"golang.org/x/term"
)

//go:embed templates/*.json
var startupTemplatesFS embed.FS

func main() {
	// Startup timing: start point = main() entry.
	// Ready point = HTTP listener bound (see "mesa ready" log below).
	startTime := time.Now()

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
	teamTemplatesDir := os.Getenv("TEAM_TEMPLATES")
	templateName := envOr("TEMPLATE", "startup")
	archetypes.SetOverridesDir(archetypesDir)

	defaultModel := envOr("MODEL", "claude")

	var templateProvided, modelProvided bool

	verbosity := 0

	dashboardAuth := false
	externalAPIKey := ""

	// CLI: mesa [-t <template>] [-m <model>] [-v|-vv|-vvv] [--auth] [--api-key <key>] [doctor|wiki-search] [port]
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			fmt.Println("Usage: mesa [-t <template>] [-m <model>] [-v|-vv|-vvv] [--auth] [--api-key <key>] [doctor|wiki-search] [port]")
			fmt.Println("  -t, --template  Team template: startup, dev-team, enterprise, saas, agency, smm, media (default: startup)")
			fmt.Println("  -m, --model     Default agent runner: claude, gemini, codex, opencode (default: claude)")
			fmt.Println("  -v              Verbosity: -v info, -vv debug, -vvv debug+cmd")
			fmt.Println("  --auth          Enable dashboard authentication with auto-generated token")
			fmt.Println("  --api-key       Static API key for external access (non-expiring, not agent-scoped)")
			fmt.Println("  doctor          Check that required CLI binaries are available")
			fmt.Println("  wiki-search     Search wiki pages (usage: wiki-search <query>)")
			fmt.Println("  port            HTTP port (default: 3001, or PORT env)")
			os.Exit(0)
		} else if arg == "doctor" {
			runDoctor()
			os.Exit(0)
		} else if arg == "wiki-search" {
			query := strings.Join(args[i+1:], " ")
			runWikiSearch(dbPath, query)
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
		} else if arg == "--auth" {
			dashboardAuth = true
		} else if arg == "--api-key" {
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --api-key requires a value")
				os.Exit(1)
			}
			i++
			externalAPIKey = args[i]
		} else {
			port = arg
		}
	}

	// Also allow env override for external API key
	if k := os.Getenv("MESA_API_KEY_EXTERNAL"); k != "" {
		externalAPIKey = k
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

	go models.DiscoverOpenCodeModels()

	// SSE hub
	sse := handlers.NewSSEHub()

	// Scheduler
	portInt := 3001
	if p, err := parsePort(port); err == nil {
		portInt = p
	}
	sched := scheduler.New(database, portInt)

	for _, bs := range scheduler.CheckBinaries() {
		if !bs.Found {
			slog.Debug("binary not found", "binary", bs.Binary, "runner", bs.Runner)
		}
	}

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
	if externalAPIKey != "" {
		api.SetExternalKey(externalAPIKey)
	}
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
	mux.HandleFunc("POST /issues/clear-open", ui.ClearOpenIssues)
	mux.HandleFunc("GET /wiki", ui.WikiList)
	mux.HandleFunc("GET /wiki/new", ui.WikiNew)
	mux.HandleFunc("POST /wiki", ui.WikiCreate)
	mux.HandleFunc("GET /wiki/{slug}", ui.WikiView)
	mux.HandleFunc("GET /wiki/{slug}/edit", ui.WikiEdit)
	mux.HandleFunc("POST /wiki/{slug}", ui.WikiUpdate)
	mux.HandleFunc("POST /wiki/{slug}/delete", ui.WikiDelete)
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
	mux.HandleFunc("GET /api/v1/issues", api.Auth(api.ListIssues))
	mux.HandleFunc("GET /api/v1/issues/{key}", api.Auth(api.GetIssue))
	mux.HandleFunc("POST /api/v1/issues/{key}/checkout", api.Auth(api.CheckoutIssue))
	mux.HandleFunc("PATCH /api/v1/issues/{key}", api.Auth(api.UpdateIssue))
	mux.HandleFunc("DELETE /api/v1/issues/{key}", api.Auth(api.DeleteIssue))
	mux.HandleFunc("POST /api/v1/issues/{key}/comments", api.Auth(api.CreateComment))
	mux.HandleFunc("POST /api/v1/issues/similarity", api.Auth(api.SimilarIssues))
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
	mux.HandleFunc("GET /api/v1/wiki/search", api.Auth(api.SearchWikiPages))
	mux.HandleFunc("GET /api/v1/wiki", api.Auth(api.ListWikiPages))
	mux.HandleFunc("POST /api/v1/wiki", api.Auth(api.CreateWikiPage))
	mux.HandleFunc("GET /api/v1/wiki/{slug}", api.Auth(api.GetWikiPage))
	mux.HandleFunc("PATCH /api/v1/wiki/{slug}", api.Auth(api.UpdateWikiPage))
	mux.HandleFunc("DELETE /api/v1/wiki/{slug}", api.Auth(api.DeleteWikiPage))

	mux.HandleFunc("GET /api/v1/apex-blocks", api.Auth(api.ListApexBlocks))
	mux.HandleFunc("GET /api/v1/apex-blocks/{id}", api.Auth(api.GetApexBlock))
	mux.HandleFunc("POST /api/v1/apex-blocks", api.Auth(api.CreateApexBlock))
	mux.HandleFunc("PATCH /api/v1/apex-blocks/{id}", api.Auth(api.UpdateApexBlock))
	mux.HandleFunc("POST /api/v1/archetype-patches", api.Auth(api.CreateArchetypePatch))

	// Version check (no auth required)
	mux.HandleFunc("GET /api/check-for-updates", ui.CheckForUpdates)

	// Webhook routes (no auth required, uses HMAC signature)
	webhookAPI := handlers.NewWebhookAPI(database, sse)
	mux.HandleFunc("POST /api/v1/webhooks/{source}/issues", webhookAPI.WebhookAuth(webhookAPI.HandleIssues))
	mux.HandleFunc("POST /api/v1/webhooks/{source}/comments", webhookAPI.WebhookAuth(webhookAPI.HandleComments))

	// Apply org template on first run
	templateName, defaultModel = promptFirstRun(database, templateName, defaultModel, templateProvided, modelProvided, teamTemplatesDir)
	applyStartupTemplate(database, templateName, defaultModel, teamTemplatesDir)

	// Recover stuck issues from previous run
	if recovered := sched.RecoverStuckIssues(); recovered > 0 {
		slog.Info("startup: recovered stuck issues", "count", recovered)
	}

	// Heartbeat loop
	sched.StartHeartbeatLoop(1 * time.Hour)

	// Cron job loop
	sched.StartCronLoop()

	// Dashboard auth
	var dashToken string
	if dashboardAuth {
		dashToken = handlers.GenerateDashboardToken()
	}
	// Also allow DASHBOARD_TOKEN env to enable auth without CLI flag
	if t := os.Getenv("DASHBOARD_TOKEN"); t != "" {
		dashToken = t
	}
	handler := handlers.AccessLog(handlers.DashboardAuth(dashToken, mux))

	// HTTP server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // disabled for SSE
	}

	if dashToken != "" {
		fmt.Fprintf(os.Stderr, "\n  Dashboard auth enabled\n")
		fmt.Fprintf(os.Stderr, "  Open: http://localhost:%s/dashboard?token=%s\n\n", port, dashToken)
	}
	if externalAPIKey != "" {
		fmt.Fprintf(os.Stderr, "  External API key enabled\n")
		fmt.Fprintf(os.Stderr, "  Usage: curl -H 'Authorization: Bearer %s' http://localhost:%s/api/v1/issues\n\n", externalAPIKey, port)
	}

	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		slog.Error("failed to bind listener", "addr", srv.Addr, "error", err)
		os.Exit(1)
	}
	if dashToken == "" {
		fmt.Fprintf(os.Stderr, "\n  mesa ready\n  Open: http://localhost:%s\n\n", port)
	}
	slog.Info("mesa ready",
		"url", "http://localhost:"+port,
		"port", port,
		"template", templateName,
		"runner", resolveRunner(defaultModel),
		"startup_duration_ms", time.Since(startTime).Milliseconds(),
	)

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
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

func applyStartupTemplate(database *db.DB, templateName, defaultModel, teamTemplatesDir string) {
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

	var data []byte
	if teamTemplatesDir != "" {
		data, err = os.ReadFile(filepath.Join(teamTemplatesDir, templateName+".json"))
	}
	if data == nil {
		data, err = startupTemplatesFS.ReadFile("templates/" + templateName + ".json")
	}
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
			HeartbeatEnabled     bool     `json:"heartbeat_enabled"`
			ChromeEnabled        bool     `json:"chrome_enabled"`
			DisableSlashCommands bool     `json:"disable_slash_commands"`
			DisableSkills        bool     `json:"disable_skills"`
			DisallowedTools      []string `json:"disallowed_tools"`
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
			HeartbeatEnabled:     a.HeartbeatEnabled,
			ChromeEnabled:        a.ChromeEnabled,
			DisableSlashCommands: a.DisableSlashCommands,
			DisableSkills:        a.DisableSkills,
			DisallowedTools:      a.DisallowedTools,
			Active:               true,
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

func promptFirstRun(database *db.DB, templateName, defaultModel string, templateProvided, modelProvided bool, teamTemplatesDir string) (string, string) {
	agents, _ := database.ListAgents()
	if len(agents) > 0 {
		return templateName, defaultModel
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return templateName, defaultModel
	}

	reader := bufio.NewReader(os.Stdin)

	// Discover custom templates from the override directory
	var customTemplates []string
	if teamTemplatesDir != "" {
		if entries, err := os.ReadDir(teamTemplatesDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					name := strings.TrimSuffix(e.Name(), ".json")
					// Skip names that match built-in templates
					switch name {
					case "startup", "dev-team", "saas", "agency", "enterprise", "smm", "media":
						continue
					}
					customTemplates = append(customTemplates, name)
				}
			}
		}
	}

	if !templateProvided {
		fmt.Println("\nSelect a team template:")
		fmt.Println("  1. startup     - Founding team: CEO, Engineer, Product, Designer, QA, DevOps")
		fmt.Println("  2. dev-team    - Engineering-focused team")
		fmt.Println("  3. saas        - SaaS product team")
		fmt.Println("  4. agency      - Agency delivery team")
		fmt.Println("  5. enterprise  - Larger org structure")
		fmt.Println("  6. smm         - Social media marketing team")
		fmt.Println("  7. media       - PR & media relations team")
		fmt.Println("  8. blank       - No agents, configure manually")
		for i, name := range customTemplates {
			fmt.Printf("  %d. %-12s - Custom template\n", 9+i, name)
		}

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
			case "6", "smm":
				return "smm"
			case "7", "media":
				return "media"
			case "8", "blank":
				return "blank"
			default:
				// Check if input matches a custom template by number or name
				for i, name := range customTemplates {
					if input == fmt.Sprintf("%d", 9+i) || input == name {
						return name
					}
				}
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

func runWikiSearch(dbPath, query string) {
	if query == "" {
		fmt.Fprintln(os.Stderr, "usage: mesa wiki-search <query>")
		os.Exit(1)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot open database %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer database.Close()

	results, err := database.SearchWikiPagesFTS(query, 20)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search error: %v\n", err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("No results found.")
		return
	}
	for _, r := range results {
		fmt.Printf("  %-30s  /wiki/%s\n", r.Title, r.Slug)
		if r.Snippet != "" {
			fmt.Printf("    %s\n", r.Snippet)
		}
	}
	fmt.Printf("\n%d result(s)\n", len(results))
}

func runDoctor() {
	fmt.Println("\033[1mmesa doctor\033[0m")
	fmt.Println("==================")
	fmt.Println()

	results := scheduler.CheckBinaries()

	allOK := true
	for _, r := range results {
		if r.Found {
			fmt.Printf("  \033[32m\u2713\033[0m  %-12s  %-10s  \033[2m%s\033[0m\n", r.Binary, r.Runner, r.Path)
		} else {
			allOK = false
			fmt.Printf("  \033[31m\u2717\033[0m  %-12s  %-10s  \033[31mnot found in PATH\033[0m\n", r.Binary, r.Runner)
		}
	}

	fmt.Println()
	if allOK {
		fmt.Println("\033[32mAll binaries found.\033[0m")
	} else {
		fmt.Println("\033[33mSome binaries are missing. Install them or ensure they are on your PATH.\033[0m")
	}
}
