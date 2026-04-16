package handlers

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/msoedov/mesa/internal/db"
	"github.com/msoedov/mesa/internal/models"
)

func LogActivityAndBroadcast(database *db.DB, sse *SSEHub, tmpl *template.Template, action, entityType, entityID string, agentID *string, details string) error {
	// 1. Save to DB
	err := database.LogActivity(action, entityType, entityID, agentID, details)
	if err != nil {
		return err
	}

	// 2. Broadcast via SSE if we have a template
	if sse != nil && tmpl != nil {
		log := models.ActivityLog{
			Action:     action,
			EntityType: entityType,
			EntityID:   entityID,
			AgentID:    agentID,
			Details:    details,
			CreatedAt:  time.Now().UTC(),
		}

		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "activity_entry", log); err == nil {
			sse.Broadcast("activity_log_created", buf.String())
		} else {
			fmt.Printf("Error executing activity_entry template: %v\n", err)
		}
	}

	return nil
}
