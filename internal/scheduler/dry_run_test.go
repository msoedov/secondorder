package scheduler

import (
	"context"
	"testing"

	"github.com/msoedov/mesa/internal/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDryRunRunner(t *testing.T) {
	Convey("Given a scheduler and an agent configured for dry_run", t, func() {
		d := testDB(t)
		s := New(d, 9102)
		agent := &models.Agent{ID: "a2", Name: "dry-bot", Runner: models.RunnerDryRun, Model: "dry_run"}

		Convey("execDryRun records the prompt and emits a dry_run envelope", func() {
			stdout, err := s.execDryRun(context.Background(), agent, "key", "run-dry-1", "SO-2", "hello prompt")

			So(err, ShouldBeNil)
			So(stdout, ShouldContainSubstring, `"type":"result"`)
			So(stdout, ShouldContainSubstring, `"type":"dry_run"`)
			So(stdout, ShouldContainSubstring, `"prompt":"hello prompt"`)
			So(stdout, ShouldContainSubstring, "SO-2")
		})

		Convey("Token parsing yields zeros so dry runs are free", func() {
			stdout, _ := s.execDryRun(context.Background(), agent, "key", "run-dry-2", "SO-3", "another prompt")
			u := parseTokenUsage(stdout)
			So(u.InputTokens, ShouldEqual, 0)
			So(u.OutputTokens, ShouldEqual, 0)
			So(u.TotalCostUSD, ShouldEqual, 0)
		})

		Convey("A cancelled context is honored", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := s.execDryRun(ctx, agent, "key", "run-dry-3", "SO-4", "prompt")
			So(err, ShouldEqual, context.Canceled)
		})
	})

	Convey("Runner registration", t, func() {
		So(models.IsValidModelForRunner(models.RunnerDryRun, "dry_run"), ShouldBeTrue)
		So(CheckRunnerBinary(models.RunnerDryRun), ShouldBeNil)
	})
}
