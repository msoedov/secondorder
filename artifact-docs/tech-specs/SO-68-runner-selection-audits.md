# Tech Spec: SO-68 Enable runner selection/configuration for audits

## Context
Users want to be able to select the AI runner (Gemini, Claude, etc.) and specific model used for audits. This should be configurable both via the UI and via a configuration file in the project root.

## Changes

### Backend
-   Modified `internal/scheduler/scheduler.go`:
    -   Updated `RunAudit` to support both `.secondorder.json` and `.secondorder.yml` configuration files.
    -   Improved configuration loading to try `.secondorder.json` first, then fall back to `.secondorder.yml`.
    -   Added validation for runner/model combinations.
    -   If only a runner is specified (via UI or config), it now defaults to the first valid model for that runner instead of the auditor agent's default model (which might be incompatible).
    -   Added final validation using `models.IsValidModelForRunner` with a fallback to auditor agent defaults if the combination is invalid.

### Frontend
-   Modified `internal/templates/policies.html`:
    -   Added `data-default-label="(Auditor Default)"` to the `audit-model` select to support the "Auditor Default" option even after selecting a runner.
-   Modified `internal/templates/partials.html`:
    -   Updated `updateModels` JavaScript function to respect `data-default-label`.
    -   Ensured that the empty option (representing the default) is added back to the model select when it's updated.

## Configuration File Example
A `.secondorder.json` or `.secondorder.yml` file in the project root can configure the default audit runner and model:

```json
{
  "audit": {
    "runner": "gemini",
    "model": "gemini-1.5-pro"
  }
}
```

## Verification Plan
1.  Run the tests in `internal/scheduler/scheduler_test.go` to ensure no regressions and verify the new configuration loading logic.
2.  Manual verification of the UI on the `/policies` page:
    -   Select a runner and verify that "(Auditor Default)" is still an option in the model select.
    -   Select a runner and leave model as "(Auditor Default)", then run audit. Verify it uses a valid default model for that runner.
