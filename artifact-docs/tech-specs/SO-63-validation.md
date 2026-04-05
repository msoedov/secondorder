# Backend Validation for Agent Runners and Models (SO-63)

## Overview
To ensure system stability and prevent misconfiguration, we have implemented server-side validation for agent models and runners. Each runner supports a specific set of models.

## Implementation Details

### Data Mapping
The mapping between runners and models is defined in `internal/models/models.go` using a map `RunnerModels`.

| Runner | Supported Models |
|--------|------------------|
| `claude_code` | `sonnet`, `opus`, `haiku` |
| `gemini` | `gemini-2.0-flash`, `gemini-2.0-flash-lite`, `gemini-1.5-pro`, `gemini-1.5-flash` |
| `codex` | `gpt-4o`, `o4-mini` |
| `antigravity` | `default` |

### Validation Logic
A new function `IsValidModelForRunner(runner, model string) bool` has been added to the `models` package. This function checks if the provided model is compatible with the selected runner.

### API/UI Integration
The validation is enforced in the following handlers in `internal/handlers/ui.go`:
- `createAgentUI`: Validates model/runner compatibility before creating a new agent.
- `updateAgentUI`: Validates model/runner compatibility before updating an existing agent.

If an invalid combination is provided, the server returns a `400 Bad Request` status with a descriptive error message.

## Verification
Automated tests have been added to `internal/handlers/handlers_test.go` in the `TestAgentUI_Validation` function to verify:
- Valid model/runner combinations for all supported runners.
- Invalid model/runner combinations for creation.
- Invalid model/runner combinations for updates.
