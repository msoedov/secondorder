package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/msoedov/thelastorg/internal/db"
	"github.com/msoedov/thelastorg/internal/handlers"
	"github.com/msoedov/thelastorg/internal/models"
	"github.com/msoedov/thelastorg/internal/scheduler"
	"github.com/msoedov/thelastorg/internal/telegram"
	"github.com/msoedov/thelastorg/internal/templates"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetLevel(log.InfoLevel)

	port := envOr("PORT", "3001")
	dbPath := envOr("DB", "tlo.db")
	archetypesDir := envOr("ARCHETYPES", "archetypes")

	// CLI: thelastorg [port]
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "-h" || arg == "--help" {
			fmt.Println("Usage: thelastorg [port]")
			fmt.Println("  port  HTTP port (default: 3001, or PORT env)")
			os.Exit(0)
		}
		port = arg
	}

	// Database
	database, err := db.Open(dbPath)
	if err != nil {
		log.WithError(err).Fatal("failed to open database")
	}

	// HTML templates
	tmpl, err := templates.Parse()
	if err != nil {
		log.WithError(err).Fatal("failed to parse templates")
	}

	// SSE hub
	sse := handlers.NewSSEHub()

	// Scheduler
	portInt := 3001
	if p, err := parsePort(port); err == nil {
		portInt = p
	}
	sched := scheduler.New(database, portInt, archetypesDir)

	// Wire wake function
	wake := sched.WakeAgent

	// Callbacks
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

	// Telegram bot (optional)
	var tg handlers.TelegramNotifier
	if token := os.Getenv("TELEGRAM_TOKEN"); token != "" {
		chatID := os.Getenv("TELEGRAM_CHAT_ID")
		bot := telegram.New(token, chatID)
		bot.OnApproval = func(blockID, decision string) {
			if decision == "approve" {
				wb, err := database.GetWorkBlock(blockID)
				if err != nil {
					log.WithError(err).Error("telegram: block not found")
					return
				}
				switch wb.Status {
				case models.WBStatusProposed:
					database.UpdateWorkBlockStatus(blockID, models.WBStatusActive)
					log.WithField("block", wb.Title).Info("telegram: block activated")
					if ceo, err := database.GetCEOAgent(); err == nil {
						sched.WakeAgentHeartbeat(ceo)
					}
				case models.WBStatusReady:
					database.UpdateWorkBlockStatus(blockID, models.WBStatusShipped)
					log.WithField("block", wb.Title).Info("telegram: block shipped")
				}
			} else {
				database.LogActivity("rejected", "work_block", blockID, nil, "Rejected via Telegram")
				log.WithField("block", blockID).Info("telegram: block rejected")
			}
		}
		ctx, tgCancel := context.WithCancel(context.Background())
		go bot.StartPolling(ctx)
		defer tgCancel()
		tg = bot
		log.Info("telegram bot enabled")
	}

	// Handlers
	api := handlers.NewAPI(database, sse, wake, tg)
	ui := handlers.NewUI(database, sse, tmpl, wake, sched)

	// Routes
	mux := http.NewServeMux()

	// UI routes
	mux.HandleFunc("GET /dashboard", ui.Dashboard)
	mux.HandleFunc("GET /issues", ui.ListIssues)
	mux.HandleFunc("POST /issues", ui.ListIssues)
	mux.HandleFunc("GET /issues/{key}", ui.IssueDetail)
	mux.HandleFunc("POST /issues/{key}", ui.IssueDetail)
	mux.HandleFunc("GET /agents", ui.ListAgents)
	mux.HandleFunc("POST /agents", ui.ListAgents)
	mux.HandleFunc("GET /agents/{slug}", ui.AgentDetail)
	mux.HandleFunc("POST /agents/{slug}", ui.AgentDetail)
	mux.HandleFunc("POST /agents/{slug}/heartbeat", ui.AgentHeartbeat)
	mux.HandleFunc("POST /agents/{slug}/assign", ui.AgentAssign)
	mux.HandleFunc("GET /work-blocks", ui.ListWorkBlocks)
	mux.HandleFunc("POST /work-blocks", ui.ListWorkBlocks)
	mux.HandleFunc("GET /work-blocks/{id}", ui.WorkBlockDetail)
	mux.HandleFunc("POST /work-blocks/{id}", ui.WorkBlockDetail)
	mux.HandleFunc("GET /activity", ui.ActivityPage)
	mux.HandleFunc("GET /policies", ui.PoliciesPage)
	mux.HandleFunc("POST /policies", ui.PoliciesPage)
	mux.HandleFunc("GET /settings", ui.Settings)
	mux.HandleFunc("POST /settings", ui.Settings)
	mux.HandleFunc("GET /runs/{id}", ui.RunDetail)
	mux.HandleFunc("GET /runs/{id}/stdout", ui.RunStdout)
	mux.HandleFunc("GET /search", ui.SearchIssuesAndAgents)
	mux.HandleFunc("GET /events", sse.ServeHTTP)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.svg")
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	})

	// API routes (auth-wrapped)
	mux.HandleFunc("GET /api/v1/inbox", api.Auth(api.Inbox))
	mux.HandleFunc("GET /api/v1/issues/{key}", api.Auth(api.GetIssue))
	mux.HandleFunc("POST /api/v1/issues/{key}/checkout", api.Auth(api.CheckoutIssue))
	mux.HandleFunc("PATCH /api/v1/issues/{key}", api.Auth(api.UpdateIssue))
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
	mux.HandleFunc("POST /api/v1/archetype-patches", api.Auth(api.CreateArchetypePatch))

	// Apply org template on first run
	applyStartupTemplate(database)

	// Heartbeat loop
	sched.StartHeartbeatLoop(5 * time.Minute)

	// HTTP server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // disabled for SSE
	}

	go func() {
		log.Infof("thelastorg running at http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("http server error")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down...")

	sse.Close()
	sched.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.WithError(err).Error("http shutdown error")
	}

	database.Close()
	log.Info("shutdown complete")
}

func applyStartupTemplate(database *db.DB) {
	agents, err := database.ListAgents()
	if err != nil {
		log.WithError(err).Error("failed to check agents table")
		return
	}
	if len(agents) > 0 {
		return
	}

	data, err := os.ReadFile("cmd/thelastorg/templates/startup.json")
	if err != nil {
		log.WithError(err).Warn("startup template not found, skipping")
		return
	}

	var tmpl struct {
		Name   string `json:"name"`
		Agents []struct {
			Name             string `json:"name"`
			Slug             string `json:"slug"`
			ArchetypeSlug    string `json:"archetype_slug"`
			Model            string `json:"model"`
			HeartbeatEnabled bool   `json:"heartbeat_enabled"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &tmpl); err != nil {
		log.WithError(err).Error("failed to parse startup template")
		return
	}

	log.Infof("applying org template: %s (%d agents)", tmpl.Name, len(tmpl.Agents))

	for _, a := range tmpl.Agents {
		agent := &models.Agent{
			ID:               uuid.New().String(),
			Name:             a.Name,
			Slug:             a.Slug,
			ArchetypeSlug:    a.ArchetypeSlug,
			Model:            a.Model,
			WorkingDir:       ".",
			MaxTurns:         50,
			TimeoutSec:       600,
			HeartbeatEnabled: a.HeartbeatEnabled,
			Active:           true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if err := database.CreateAgent(agent); err != nil {
			log.WithError(err).Errorf("failed to create agent: %s", a.Name)
			continue
		}
		log.Infof("created agent: %s (slug=%s, archetype=%s, model=%s)", a.Name, a.Slug, a.ArchetypeSlug, a.Model)
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
