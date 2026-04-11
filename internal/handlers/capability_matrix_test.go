package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

type capabilityStubTelegram struct{}

func (s *capabilityStubTelegram) SendWorkBlockApproval(_, _, _, _ string) error { return nil }
func (s *capabilityStubTelegram) SendMessage(_ string) error                    { return nil }

func capabilityTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestAgentCapabilityMatrixHappyPath(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	workingDir := t.TempDir()
	t.Setenv("BACKEND_AGENT_TOKEN", "present")

	agent := &models.Agent{
		Name:          "Backend Engineer",
		Slug:          "backend-engineer",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "BACKEND_AGENT_TOKEN",
		WorkingDir:    workingDir,
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: true,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := d.SetSetting("instance_name", "test-instance"); err != nil {
		t.Fatalf("set instance setting: %v", err)
	}

	rawKey := "so_test_capability_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-capability-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Run.InstanceName != "test-instance" {
		t.Fatalf("instance_name = %q, want test-instance", resp.Run.InstanceName)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(resp.Agents))
	}

	row := resp.Agents[0]
	if row.AgentSlug != "backend-engineer" {
		t.Fatalf("agent_slug = %q, want backend-engineer", row.AgentSlug)
	}

	cred := findCredential(t, row.Credentials, "cred:backend-engineer:primary_api_key")
	if cred.Status != "verified" {
		t.Fatalf("credential status = %q, want verified", cred.Status)
	}

	workspace := findCapability(t, row.EnvironmentCapabilities, "workspace_access")
	if workspace.Status != "verified" {
		t.Fatalf("workspace status = %q, want verified", workspace.Status)
	}

	patchAction := findCapability(t, row.Capabilities, "archetype_patch_submission")
	if patchAction.Status != "verified" {
		t.Fatalf("patch submission status = %q, want verified", patchAction.Status)
	}

	mergeAction := findCapability(t, row.Capabilities, "merge_pull_request")
	if mergeAction.Status != "unknown" {
		t.Fatalf("merge status = %q, want unknown", mergeAction.Status)
	}
}

func TestAgentCapabilityMatrixUnknownAndUnavailableStates(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "QA Engineer",
		Slug:          "qa-engineer",
		ArchetypeSlug: "qa-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "QA_AGENT_TOKEN",
		WorkingDir:    "/does/not/exist",
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: false,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	requester := &models.Agent{
		Name:          "Requester",
		Slug:          "requester",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "REQUESTER_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: false,
		Active:        true,
	}
	if err := d.CreateAgent(requester); err != nil {
		t.Fatalf("create requester: %v", err)
	}

	rawKey := "so_test_capability_key_unknown"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(requester.ID, "run-capability-2", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	qaRow := findAgentRow(t, resp.Agents, "qa-engineer")
	cred := findCredential(t, qaRow.Credentials, "cred:qa-engineer:primary_api_key")
	if cred.Status != "unknown" {
		t.Fatalf("credential status = %q, want unknown", cred.Status)
	}

	workspace := findCapability(t, qaRow.EnvironmentCapabilities, "workspace_access")
	if workspace.Status != "unavailable" {
		t.Fatalf("workspace status = %q, want unavailable", workspace.Status)
	}

	chrome := findCapability(t, qaRow.EnvironmentCapabilities, "chrome_mcp_access")
	if chrome.Status != "unavailable" {
		t.Fatalf("chrome status = %q, want unavailable", chrome.Status)
	}
}

func TestAgentCapabilityMatrixContractEndpoint(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "Contract Reader",
		Slug:          "contract-reader",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "CONTRACT_READER_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_capability_key_contract"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-capability-3", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix/contract", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrixContract)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	if _, ok := payload["status_values"]["unknown"]; !ok {
		t.Fatalf("contract missing unknown status description")
	}
}

// TestAgentCapabilityMatrixRequiresAuth verifies the endpoint rejects unauthenticated requests.
func TestAgentCapabilityMatrixRequiresAuth(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)

	for _, path := range []string{
		"/api/v1/agents/capability-matrix",
		"/api/v1/agents/capability-matrix/contract",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		var handler http.HandlerFunc
		if path == "/api/v1/agents/capability-matrix" {
			handler = api.Auth(api.AgentCapabilityMatrix)
		} else {
			handler = api.Auth(api.AgentCapabilityMatrixContract)
		}
		handler(w, req)

		if w.Code == http.StatusOK {
			t.Fatalf("path %s: expected non-200 for unauthenticated request, got %d", path, w.Code)
		}
	}
}

// TestAgentCapabilityMatrixNoSecretMaterialLeaked verifies no credential values,
// env var names, or bearer tokens appear in the response body.
func TestAgentCapabilityMatrixNoSecretMaterialLeaked(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	secretEnvName := "SUPER_SECRET_AGENT_TOKEN_LEAK_TEST"
	secretEnvValue := "plaintext-secret-value-abc123"
	t.Setenv(secretEnvName, secretEnvValue)

	agent := &models.Agent{
		Name:          "Secret Agent",
		Slug:          "secret-agent",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     secretEnvName,
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_no_leak_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-noleak-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	body := w.Body.String()

	// The secret env var value must never appear in the response
	if contains(body, secretEnvValue) {
		t.Fatalf("response body contains secret env var value: %q", secretEnvValue)
	}

	// The raw bearer token used for auth must never appear in the response
	if contains(body, rawKey) {
		t.Fatalf("response body contains raw bearer token: %q", rawKey)
	}

	// The env var name itself (which could reveal internal topology) must not appear
	if contains(body, secretEnvName) {
		t.Fatalf("response body contains secret env var name: %q", secretEnvName)
	}
}

// TestAgentCapabilityMatrixUnavailableWhenApiKeyEnvUnset confirms that an agent
// with an empty ApiKeyEnv gets "unavailable" (not "unknown") for the credential.
func TestAgentCapabilityMatrixUnavailableWhenApiKeyEnvUnset(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "No Cred Agent",
		Slug:          "no-cred-agent",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "", // empty — no env var configured
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	requester := &models.Agent{
		Name:          "Requester Unset",
		Slug:          "requester-unset",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "REQUESTER_UNSET_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	t.Setenv("REQUESTER_UNSET_TOKEN", "present")
	if err := d.CreateAgent(requester); err != nil {
		t.Fatalf("create requester: %v", err)
	}

	rawKey := "so_test_unset_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(requester.ID, "run-unset-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	row := findAgentRow(t, resp.Agents, "no-cred-agent")
	cred := findCredential(t, row.Credentials, "cred:no-cred-agent:primary_api_key")
	if cred.Status != "unavailable" {
		t.Fatalf("credential status = %q, want unavailable (api_key_env empty)", cred.Status)
	}
	if len(cred.ReasonCodes) == 0 || cred.ReasonCodes[0] != "api_key_env_unset" {
		t.Fatalf("expected reason_code api_key_env_unset, got %v", cred.ReasonCodes)
	}
}

// TestAgentCapabilityMatrixContractHasAllStatuses ensures the contract endpoint
// documents all three required status values: verified, unknown, unavailable.
func TestAgentCapabilityMatrixContractHasAllStatuses(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "Contract Check Agent",
		Slug:          "contract-check",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "CONTRACT_CHECK_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_contract_all_statuses"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-contract-all-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix/contract", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrixContract)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode contract: %v", err)
	}

	for _, required := range []string{"verified", "unknown", "unavailable"} {
		if desc, ok := payload["status_values"][required]; !ok || desc == "" {
			t.Fatalf("contract missing or empty status description for %q", required)
		}
	}
}

// TestAgentCapabilityMatrixVerificationMetadataPresent checks that all capability
// and credential entries carry populated verification metadata (level, method, source).
func TestAgentCapabilityMatrixVerificationMetadataPresent(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	workingDir := t.TempDir()
	t.Setenv("METADATA_AGENT_TOKEN", "present")

	agent := &models.Agent{
		Name:          "Metadata Agent",
		Slug:          "metadata-agent",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "METADATA_AGENT_TOKEN",
		WorkingDir:    workingDir,
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: false,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_metadata_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-metadata-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Run.Verification.Level == "" || resp.Run.Verification.Method == "" || resp.Run.Verification.Source == "" {
		t.Fatalf("run context missing verification fields: %+v", resp.Run.Verification)
	}
	if resp.Run.Verification.CheckedAt == "" {
		t.Fatalf("run context missing checked_at timestamp")
	}

	row := findAgentRow(t, resp.Agents, "metadata-agent")

	for _, cap := range row.Capabilities {
		if cap.Verification.Level == "" {
			t.Fatalf("capability %q missing verification.level", cap.Key)
		}
		if cap.Verification.Method == "" {
			t.Fatalf("capability %q missing verification.method", cap.Key)
		}
		if cap.Verification.Source == "" {
			t.Fatalf("capability %q missing verification.source", cap.Key)
		}
		if cap.Verification.CheckedAt == "" {
			t.Fatalf("capability %q missing verification.checked_at", cap.Key)
		}
	}

	for _, env := range row.EnvironmentCapabilities {
		if env.Verification.Level == "" {
			t.Fatalf("env capability %q missing verification.level", env.Key)
		}
		if env.Verification.CheckedAt == "" {
			t.Fatalf("env capability %q missing verification.checked_at", env.Key)
		}
	}

	for _, cred := range row.Credentials {
		if cred.Verification.Level == "" {
			t.Fatalf("credential %q missing verification.level", cred.Ref)
		}
		if cred.Verification.CheckedAt == "" {
			t.Fatalf("credential %q missing verification.checked_at", cred.Ref)
		}
	}
}

// TestAgentCapabilityMatrixChromeMCPVerifiedWhenEnabled confirms that
// chrome_enabled=true results in a "verified" chrome_mcp_access status.
func TestAgentCapabilityMatrixChromeMCPVerifiedWhenEnabled(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "Chrome Agent",
		Slug:          "chrome-agent",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "CHROME_AGENT_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: true, // explicitly enabled
		Active:        true,
	}
	t.Setenv("CHROME_AGENT_TOKEN", "present")
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_chrome_verified_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-chrome-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	row := findAgentRow(t, resp.Agents, "chrome-agent")
	chrome := findCapability(t, row.EnvironmentCapabilities, "chrome_mcp_access")
	if chrome.Status != "verified" {
		t.Fatalf("chrome_mcp_access status = %q, want verified when chrome_enabled=true", chrome.Status)
	}
	if len(chrome.ReasonCodes) != 0 {
		t.Fatalf("expected no reason_codes when chrome verified, got %v", chrome.ReasonCodes)
	}
}

// TestAgentCapabilityMatrixRunContextFields validates required run context fields
// including generated_at_utc, instance_name, and running_runs_count.
func TestAgentCapabilityMatrixRunContextFields(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	if err := d.SetSetting("instance_name", "qa-test-instance"); err != nil {
		t.Fatalf("set instance setting: %v", err)
	}

	agent := &models.Agent{
		Name:          "Run Context Agent",
		Slug:          "run-context-agent",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "RC_AGENT_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	t.Setenv("RC_AGENT_TOKEN", "present")
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_run_context_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-rc-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Validate run context fields
	if resp.Run.InstanceName != "qa-test-instance" {
		t.Fatalf("instance_name = %q, want qa-test-instance", resp.Run.InstanceName)
	}
	if resp.Run.GeneratedAtUTC == "" {
		t.Fatalf("generated_at_utc must be non-empty")
	}
	// Validate it parses as RFC3339
	if _, err := time.Parse(time.RFC3339, resp.Run.GeneratedAtUTC); err != nil {
		t.Fatalf("generated_at_utc %q is not valid RFC3339: %v", resp.Run.GeneratedAtUTC, err)
	}
	// running_runs_count must be ≥0 (zero in test env is valid)
	if resp.Run.RunningRunsCount < 0 {
		t.Fatalf("running_runs_count = %d, must not be negative", resp.Run.RunningRunsCount)
	}
}

// TestAgentCapabilityMatrixCredentialRefsInCapabilities verifies that capability
// entries that depend on a credential include a non-empty credential_refs list.
func TestAgentCapabilityMatrixCredentialRefsInCapabilities(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "Cred Ref Agent",
		Slug:          "cred-ref-agent",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "CREDREF_AGENT_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: false,
		Active:        true,
	}
	t.Setenv("CREDREF_AGENT_TOKEN", "present")
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_credref_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-credref-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	row := findAgentRow(t, resp.Agents, "cred-ref-agent")

	// archetype_patch_submission requires credentials so must have non-empty credential_refs
	patchAction := findCapability(t, row.Capabilities, "archetype_patch_submission")
	if len(patchAction.CredentialRefs) == 0 {
		t.Fatalf("archetype_patch_submission should have credential_refs, got none")
	}
	// Verify credential ref format: must be cred:<slug>:<key>
	for _, ref := range patchAction.CredentialRefs {
		if !startsWith(ref, "cred:") {
			t.Fatalf("credential_ref %q does not use expected sanitized format cred:<slug>:<key>", ref)
		}
	}
}

// TestAgentCapabilityMatrixMultiAgentAllAppear confirms that when multiple agents
// exist, all of them appear in the matrix response.
func TestAgentCapabilityMatrixMultiAgentAllAppear(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agentSlugs := []string{"agent-alpha", "agent-beta", "agent-gamma"}
	for i, slug := range agentSlugs {
		envKey := "MULTI_AGENT_TOKEN_" + slug
		agent := &models.Agent{
			Name:          "Agent " + slug,
			Slug:          slug,
			ArchetypeSlug: "backend-engineer",
			Runner:        models.RunnerOpenCode,
			Model:         "default",
			ApiKeyEnv:     envKey,
			WorkingDir:    t.TempDir(),
			MaxTurns:      50,
			TimeoutSec:    1200,
			Active:        true,
		}
		t.Setenv(envKey, "present")
		if err := d.CreateAgent(agent); err != nil {
			t.Fatalf("create agent %d: %v", i, err)
		}
	}

	// Use first agent for auth
	requesterEnv := "MULTI_REQUESTER_TOKEN"
	requester := &models.Agent{
		Name:          "Requester",
		Slug:          "multi-requester",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     requesterEnv,
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	t.Setenv(requesterEnv, "present")
	if err := d.CreateAgent(requester); err != nil {
		t.Fatalf("create requester: %v", err)
	}

	rawKey := "so_test_multi_agent_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(requester.ID, "run-multi-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// All created agents plus the requester should appear
	if len(resp.Agents) < len(agentSlugs)+1 {
		t.Fatalf("expected at least %d agents in response, got %d", len(agentSlugs)+1, len(resp.Agents))
	}

	// Verify each named agent is present
	for _, slug := range agentSlugs {
		_ = findAgentRow(t, resp.Agents, slug)
	}
}

// TestAgentCapabilityMatrixWorkspaceUnknownWhenWorkingDirEmpty confirms that
// an agent with an empty working_dir gets "unknown" status for workspace_access.
func TestAgentCapabilityMatrixWorkspaceUnknownWhenWorkingDirEmpty(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "No Dir Agent",
		Slug:          "no-dir-agent",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "NO_DIR_TOKEN",
		WorkingDir:    "", // empty working dir
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	t.Setenv("NO_DIR_TOKEN", "present")
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_no_dir_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-nodir-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	row := findAgentRow(t, resp.Agents, "no-dir-agent")
	workspace := findCapability(t, row.EnvironmentCapabilities, "workspace_access")
	if workspace.Status != "unknown" {
		t.Fatalf("workspace_access status = %q, want unknown when working_dir is empty", workspace.Status)
	}
	if len(workspace.ReasonCodes) == 0 || workspace.ReasonCodes[0] != "working_dir_not_set" {
		t.Fatalf("expected reason_code working_dir_not_set, got %v", workspace.ReasonCodes)
	}
}

// startsWith is a simple prefix check helper.
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// contains is a simple substring check helper for secret-leak tests.
func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func findAgentRow(t *testing.T, rows []agentCapabilityMatrixRow, slug string) agentCapabilityMatrixRow {
	t.Helper()
	for _, row := range rows {
		if row.AgentSlug == slug {
			return row
		}
	}
	t.Fatalf("agent row not found: %s", slug)
	return agentCapabilityMatrixRow{}
}

func findCapability(t *testing.T, items []capabilityItem, key string) capabilityItem {
	t.Helper()
	for _, item := range items {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("capability not found: %s", key)
	return capabilityItem{}
}

func findCredential(t *testing.T, items []credentialItem, ref string) credentialItem {
	t.Helper()
	for _, item := range items {
		if item.Ref == ref {
			return item
		}
	}
	t.Fatalf("credential not found: %s", ref)
	return credentialItem{}
}
