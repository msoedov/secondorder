package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/msoedov/mesa/internal/db"
	"github.com/msoedov/mesa/internal/models"
	"github.com/msoedov/mesa/internal/scheduler"
	"github.com/msoedov/mesa/internal/templates"
	. "github.com/smartystreets/goconvey/convey"
)

// freshDB returns an isolated in-memory DB so each Convey leaf starts clean.
func freshDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString()))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// waitForRun polls for a completed run up to d, then fails the Convey assertion.
func waitForRun(database *db.DB, agentID string, d time.Duration) *models.Run {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		runs, _ := database.ListRunsForAgent(agentID, 1)
		if len(runs) > 0 && runs[0].Status == models.RunStatusCompleted {
			return &runs[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

func TestHTTPPagesWithNoopRunner(t *testing.T) {
	Convey("Given a UI wired to a scheduler with a noop-runner agent", t, func() {
		d := freshDB(t)
		sse := NewSSEHub()
		t.Cleanup(sse.Close)

		tmpl, err := templates.Parse()
		So(err, ShouldBeNil)

		sched := scheduler.New(d, 0)
		ui := NewUI(d, sse, tmpl, sched.WakeAgent, sched)

		agent := &models.Agent{
			Name: "Noop Bot", Slug: "noop-bot", ArchetypeSlug: "worker",
			Runner: models.RunnerNoop, Model: "noop",
			WorkingDir: "/tmp", MaxTurns: 1, TimeoutSec: 10, Active: true,
		}
		So(d.CreateAgent(agent), ShouldBeNil)

		issue := &models.Issue{
			Key: "SO-NOOP-1", Title: "smoke-test issue",
			Description: "seeded by noop test", Status: models.StatusTodo,
			AssigneeAgentID: &agent.ID,
		}
		So(d.CreateIssue(issue), ShouldBeNil)

		Convey("The dashboard renders 200 and lists the agent", func() {
			req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
			rr := httptest.NewRecorder()
			ui.Dashboard(rr, req)

			So(rr.Code, ShouldEqual, http.StatusOK)
			So(rr.Body.String(), ShouldContainSubstring, "Noop Bot")
		})

		Convey("The agents index renders 200 and mentions the agent slug", func() {
			req := httptest.NewRequest(http.MethodGet, "/agents", nil)
			rr := httptest.NewRecorder()
			ui.ListAgents(rr, req)

			So(rr.Code, ShouldEqual, http.StatusOK)
			So(rr.Body.String(), ShouldContainSubstring, "noop-bot")
		})

		Convey("The issues index renders 200 and mentions the seeded issue", func() {
			req := httptest.NewRequest(http.MethodGet, "/issues", nil)
			rr := httptest.NewRecorder()
			ui.ListIssues(rr, req)

			So(rr.Code, ShouldEqual, http.StatusOK)
			So(rr.Body.String(), ShouldContainSubstring, "SO-NOOP-1")
		})

		Convey("Waking the agent produces a completed run (end-to-end via scheduler)", func() {
			sched.WakeAgent(agent, issue)

			run := waitForRun(d, agent.ID, 2*time.Second)
			So(run, ShouldNotBeNil)
			So(run.Status, ShouldEqual, models.RunStatusCompleted)

			Convey("and the run detail page renders without error", func() {
				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/runs/%s", run.ID), nil)
				req.SetPathValue("id", run.ID)
				rr := httptest.NewRecorder()
				ui.RunDetail(rr, req)

				So(rr.Code, ShouldEqual, http.StatusOK)
				So(rr.Body.String(), ShouldContainSubstring, "Noop Bot")
			})

			Convey("and the stored stdout contains the noop marker", func() {
				stored, err := d.GetRun(run.ID)
				So(err, ShouldBeNil)
				So(stored.Stdout, ShouldContainSubstring, `"type":"noop"`)
				So(stored.Stdout, ShouldContainSubstring, "SO-NOOP-1")
			})
		})

		// Ensure any running goroutine is drained before the DB closes.
		Reset(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			done := make(chan struct{})
			go func() { sched.Stop(); close(done) }()
			select {
			case <-done:
			case <-ctx.Done():
			}
		})
	})
}
