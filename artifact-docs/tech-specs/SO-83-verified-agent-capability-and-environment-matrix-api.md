# SO-83: Verified agent capability and environment matrix API

## Scope

Backend API support for exposing a verified capability and environment matrix in the current run context, including sanitized credential visibility, allowed external actions, and key runtime environment capabilities.

## Endpoints

- `GET /api/v1/agents/capability-matrix`
- `GET /api/v1/agents/capability-matrix/contract`

Both routes are bearer-auth protected via existing `api.Auth(...)` middleware and are wired in `cmd/secondorder/main.go`.

## Response model

`GET /api/v1/agents/capability-matrix` returns:

- `run`
  - `generated_at_utc`
  - `instance_name`
  - `running_runs_count`
  - `verification` metadata (`level`, `method`, `source`, `checked_at`, `expires_at`)
- `agents[]` (one row per agent)
  - identity fields: `agent_id`, `agent_slug`, `agent_name`, `archetype_slug`, `runner`
  - `credentials[]` (sanitized references and verification semantics)
  - `capabilities[]` (allowed/known external actions)
  - `environment_capabilities[]` (runtime environment capabilities)

`GET /api/v1/agents/capability-matrix/contract` returns explicit status semantics for clients:

- `verified`
- `unknown`
- `unavailable`

## Capability categories included

- Credentials/integrations:
  - `cred:<agent-slug>:primary_api_key`
- External actions:
  - `archetype_patch_submission` (verified via route policy)
  - `merge_pull_request` (currently unknown due to missing merge policy registry)
- Environment capabilities:
  - `workspace_access` (filesystem probe against `agents.working_dir`)
  - `chrome_mcp_access` (from `agents.chrome_enabled`)

## Verification and provenance semantics

Each capability and credential includes `verification` metadata:

- `level`: mirrors status (`verified|unknown|unavailable`)
- `method`: attestation mechanism (for example `runtime_env_presence`, `filesystem_probe`, `agent_configuration`, `api_route_policy`)
- `source`: data source identifier (for example `process_environment`, `agents.working_dir`, `agents.chrome_enabled`, route path)
- `checked_at`: RFC3339 UTC timestamp at snapshot generation
- `expires_at`: reserved/null for current implementation

## Sanitization and trust-boundary controls

- Credential values are never returned in response payloads.
- Raw env var names are not returned; credentials are represented by stable sanitized refs only.
- Route auth remains mandatory; matrix data is not publicly accessible.
- Unknown/unavailable are represented explicitly to avoid inferring missing values as denied values.

## Automated coverage

`internal/handlers/capability_matrix_test.go` covers:

- Verified states:
  - verified credential presence, workspace access, and archetype patch capability
- Unknown/unavailable states:
  - unknown credential when env value is absent
  - unavailable workspace/chrome capabilities when missing/disabled
- Contract semantics:
  - status value descriptions returned from `/contract`

## Implementation references

- `internal/handlers/capability_matrix.go`
- `internal/handlers/capability_matrix_test.go`
- `cmd/secondorder/main.go`
