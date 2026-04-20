package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/msoedov/mesa/internal/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNoopRunner(t *testing.T) {
	Convey("Given a scheduler wired to an in-memory DB", t, func() {
		d := testDB(t)
		s := New(d, 9101)
		agent := &models.Agent{ID: "a1", Name: "noop-agent", Runner: models.RunnerNoop, Model: "noop"}

		Convey("When execNoop is invoked with a fresh context", func() {
			stdout, err := s.execNoop(context.Background(), agent, "key", "run-noop-1", "SO-1", "prompt")

			Convey("It returns no error and a result line the parser understands", func() {
				So(err, ShouldBeNil)
				So(stdout, ShouldContainSubstring, `"type":"result"`)
				So(stdout, ShouldContainSubstring, `"type":"noop"`)
				So(strings.Contains(stdout, "SO-1"), ShouldBeTrue)
			})

			Convey("Token parsing yields zeros (no cost)", func() {
				u := parseTokenUsage(stdout)
				So(u.InputTokens, ShouldEqual, 0)
				So(u.OutputTokens, ShouldEqual, 0)
				So(u.TotalCostUSD, ShouldEqual, 0)
			})
		})

		Convey("When the context is already cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := s.execNoop(ctx, agent, "key", "run-noop-2", "SO-2", "prompt")

			Convey("It returns the context error", func() {
				So(err, ShouldEqual, context.Canceled)
			})
		})
	})

	Convey("Runner registration", t, func() {
		Convey("noop is advertised as a valid runner/model pair", func() {
			So(models.IsValidModelForRunner(models.RunnerNoop, "noop"), ShouldBeTrue)
		})
		Convey("noop does not require a CLI binary", func() {
			So(CheckRunnerBinary(models.RunnerNoop), ShouldBeNil)
		})
	})
}
