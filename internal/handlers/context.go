package handlers

import (
	"context"

	"github.com/msoedov/mesa/internal/models"
)

type ctxKey string

const agentKey ctxKey = "agent"

func withAgent(ctx context.Context, agent *models.Agent) context.Context {
	return context.WithValue(ctx, agentKey, agent)
}

func agentFromContext(ctx context.Context) *models.Agent {
	v, _ := ctx.Value(agentKey).(*models.Agent)
	return v
}
