package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/msoedov/secondorder/internal/models"
)

type capabilityVerification struct {
	Level     string  `json:"level"`
	Method    string  `json:"method"`
	Source    string  `json:"source"`
	CheckedAt string  `json:"checked_at"`
	ExpiresAt *string `json:"expires_at"`
}

type capabilityItem struct {
	Key            string                 `json:"key"`
	Category       string                 `json:"category"`
	DisplayName    string                 `json:"display_name"`
	Status         string                 `json:"status"`
	Scope          string                 `json:"scope"`
	Constraints    []string               `json:"constraints"`
	CredentialRefs []string               `json:"credential_refs"`
	ReasonCodes    []string               `json:"reason_codes"`
	Verification   capabilityVerification `json:"verification"`
}

type credentialItem struct {
	Ref          string                 `json:"ref"`
	Category     string                 `json:"category"`
	DisplayName  string                 `json:"display_name"`
	Status       string                 `json:"status"`
	ReasonCodes  []string               `json:"reason_codes"`
	Verification capabilityVerification `json:"verification"`
}

type agentCapabilityMatrixRow struct {
	AgentID                 string           `json:"agent_id"`
	AgentSlug               string           `json:"agent_slug"`
	AgentName               string           `json:"agent_name"`
	ArchetypeSlug           string           `json:"archetype_slug"`
	Runner                  string           `json:"runner"`
	Capabilities            []capabilityItem `json:"capabilities"`
	EnvironmentCapabilities []capabilityItem `json:"environment_capabilities"`
	Credentials             []credentialItem `json:"credentials"`
}

type capabilityRunContext struct {
	GeneratedAtUTC   string                 `json:"generated_at_utc"`
	InstanceName     string                 `json:"instance_name"`
	RunningRunsCount int                    `json:"running_runs_count"`
	Verification     capabilityVerification `json:"verification"`
}

type capabilityMatrixResponse struct {
	Run    capabilityRunContext       `json:"run"`
	Agents []agentCapabilityMatrixRow `json:"agents"`
}

func (a *API) AgentCapabilityMatrix(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339)

	instanceName := ""
	if val, err := a.db.GetSetting("instance_name"); err == nil {
		instanceName = strings.TrimSpace(val)
	}
	if instanceName == "" {
		instanceName = "unknown"
	}

	runningRunsCount, err := a.db.CountRunningRuns()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agents, err := a.db.ListAgents()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := capabilityMatrixResponse{
		Run: capabilityRunContext{
			GeneratedAtUTC:   timestamp,
			InstanceName:     instanceName,
			RunningRunsCount: runningRunsCount,
			Verification: capabilityVerification{
				Level:     "verified",
				Method:    "database_snapshot",
				Source:    "settings+runs",
				CheckedAt: timestamp,
			},
		},
		Agents: make([]agentCapabilityMatrixRow, 0, len(agents)),
	}

	for _, ag := range agents {
		resp.Agents = append(resp.Agents, buildAgentCapabilityRow(ag, timestamp))
	}

	jsonOK(w, resp)
}

func (a *API) AgentCapabilityMatrixContract(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]any{
		"status_values": map[string]string{
			"verified":    "Backend-attested fact in current run context",
			"unknown":     "Data source exists but value cannot be verified now",
			"unavailable": "Capability or credential is configured as absent or inaccessible",
		},
	})
}

func buildAgentCapabilityRow(ag models.Agent, checkedAt string) agentCapabilityMatrixRow {
	credential := buildPrimaryCredential(ag, checkedAt)

	capabilities := []capabilityItem{
		{
			Key:            "archetype_patch_submission",
			Category:       "external_action",
			DisplayName:    "Submit archetype patch",
			Status:         "verified",
			Scope:          "current_run_context",
			Constraints:    []string{"requires_authenticated_api_key"},
			CredentialRefs: []string{credential.Ref},
			ReasonCodes:    []string{},
			Verification: capabilityVerification{
				Level:     "verified",
				Method:    "api_route_policy",
				Source:    "POST /api/v1/archetype-patches",
				CheckedAt: checkedAt,
			},
		},
		{
			Key:            "merge_pull_request",
			Category:       "external_action",
			DisplayName:    "Merge pull request",
			Status:         "unknown",
			Scope:          "current_run_context",
			Constraints:    []string{},
			CredentialRefs: []string{},
			ReasonCodes:    []string{"no_merge_policy_registry"},
			Verification: capabilityVerification{
				Level:     "unknown",
				Method:    "policy_lookup",
				Source:    "not_configured",
				CheckedAt: checkedAt,
			},
		},
	}

	environment := []capabilityItem{
		buildWorkingDirCapability(ag, checkedAt),
		{
			Key:            "chrome_mcp_access",
			Category:       "environment",
			DisplayName:    "Chrome MCP access",
			Status:         boolStatus(ag.ChromeEnabled),
			Scope:          "agent_runtime",
			Constraints:    []string{},
			CredentialRefs: []string{},
			ReasonCodes:    boolReasonCodes(ag.ChromeEnabled, "chrome_disabled_for_agent"),
			Verification: capabilityVerification{
				Level:     boolStatus(ag.ChromeEnabled),
				Method:    "agent_configuration",
				Source:    "agents.chrome_enabled",
				CheckedAt: checkedAt,
			},
		},
	}

	return agentCapabilityMatrixRow{
		AgentID:                 ag.ID,
		AgentSlug:               ag.Slug,
		AgentName:               ag.Name,
		ArchetypeSlug:           ag.ArchetypeSlug,
		Runner:                  ag.Runner,
		Capabilities:            capabilities,
		EnvironmentCapabilities: environment,
		Credentials:             []credentialItem{credential},
	}
}

func buildPrimaryCredential(ag models.Agent, checkedAt string) credentialItem {
	ref := fmt.Sprintf("cred:%s:primary_api_key", ag.Slug)
	if strings.TrimSpace(ag.ApiKeyEnv) == "" {
		return credentialItem{
			Ref:         ref,
			Category:    "integration",
			DisplayName: "Primary API key",
			Status:      "unavailable",
			ReasonCodes: []string{"api_key_env_unset"},
			Verification: capabilityVerification{
				Level:     "unavailable",
				Method:    "agent_configuration",
				Source:    "agents.api_key_env",
				CheckedAt: checkedAt,
			},
		}
	}

	_, present := os.LookupEnv(ag.ApiKeyEnv)
	if !present {
		return credentialItem{
			Ref:         ref,
			Category:    "integration",
			DisplayName: "Primary API key",
			Status:      "unknown",
			ReasonCodes: []string{"credential_value_not_in_runtime_env"},
			Verification: capabilityVerification{
				Level:     "unknown",
				Method:    "runtime_env_presence",
				Source:    "process_environment",
				CheckedAt: checkedAt,
			},
		}
	}

	return credentialItem{
		Ref:         ref,
		Category:    "integration",
		DisplayName: "Primary API key",
		Status:      "verified",
		ReasonCodes: []string{},
		Verification: capabilityVerification{
			Level:     "verified",
			Method:    "runtime_env_presence",
			Source:    "process_environment",
			CheckedAt: checkedAt,
		},
	}
}

func buildWorkingDirCapability(ag models.Agent, checkedAt string) capabilityItem {
	if strings.TrimSpace(ag.WorkingDir) == "" {
		return capabilityItem{
			Key:            "workspace_access",
			Category:       "environment",
			DisplayName:    "Workspace access",
			Status:         "unknown",
			Scope:          "agent_runtime",
			Constraints:    []string{},
			CredentialRefs: []string{},
			ReasonCodes:    []string{"working_dir_not_set"},
			Verification: capabilityVerification{
				Level:     "unknown",
				Method:    "filesystem_probe",
				Source:    "agents.working_dir",
				CheckedAt: checkedAt,
			},
		}
	}

	workingDir := filepath.Clean(ag.WorkingDir)
	if _, err := os.Stat(workingDir); err != nil {
		return capabilityItem{
			Key:            "workspace_access",
			Category:       "environment",
			DisplayName:    "Workspace access",
			Status:         "unavailable",
			Scope:          "agent_runtime",
			Constraints:    []string{},
			CredentialRefs: []string{},
			ReasonCodes:    []string{"working_dir_missing"},
			Verification: capabilityVerification{
				Level:     "unavailable",
				Method:    "filesystem_probe",
				Source:    "agents.working_dir",
				CheckedAt: checkedAt,
			},
		}
	}

	return capabilityItem{
		Key:            "workspace_access",
		Category:       "environment",
		DisplayName:    "Workspace access",
		Status:         "verified",
		Scope:          "agent_runtime",
		Constraints:    []string{},
		CredentialRefs: []string{},
		ReasonCodes:    []string{},
		Verification: capabilityVerification{
			Level:     "verified",
			Method:    "filesystem_probe",
			Source:    "agents.working_dir",
			CheckedAt: checkedAt,
		},
	}
}

func boolStatus(v bool) string {
	if v {
		return "verified"
	}
	return "unavailable"
}

func boolReasonCodes(v bool, disabledReason string) []string {
	if v {
		return []string{}
	}
	return []string{disabledReason}
}
