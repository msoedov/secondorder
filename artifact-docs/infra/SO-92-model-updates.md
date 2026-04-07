# Infrastructure Update: Gemini and Codex Models (SO-92)

## Overview
Updated the runner configurations to include the latest Gemini and Codex models as of April 2026. This ensures that all agents use the most reliable and performant models available.

## Changes

### 1. Model Definitions
- **internal/models/models.go**: Updated `RunnerModels` map with:
  - **Gemini**: `gemini-3.1-pro` (flagship), `gemini-3.1-flash-lite`, `gemini-3.1-flash-live`, `gemini-3-deep-think`, `gemini-3-flash`, `gemini-3-flash-preview`.
  - **Codex**: `gpt-5.4-thinking`, `gpt-5.4-pro`, `gpt-5.4-instant`.
- **internal/templates/partials.html**: Updated `updateModels` JavaScript function to reflect these changes in the UI.

### 2. Database Migration
- **internal/db/migrations/014_update_gemini_codex_models.sql**: Added a migration to automatically update existing agents and audit runs in the database to the latest models (`gemini-3.1-pro` and `gpt-5.4-thinking`).

### 3. Test Updates
- **internal/scheduler/scheduler_test.go**: Updated tests to use the new models and verified that the default model selection (first in list) correctly picks `gemini-3.1-pro`.
- **internal/handlers/handlers_test.go**: Updated validation tests to use `gpt-5.4-thinking`.

### 4. Bug Fixes (Operational Reliability)
- **internal/handlers/ui.go**: Fixed a bug where `NorthStarMetric` and `NorthStarTarget` were not being correctly saved during Work Block creation and update. This fix was required to resolve failures in `TestStrategicAlignmentE2E`.

## Verification
- Successfully ran all tests in `internal/models`, `internal/handlers`, and `internal/scheduler`.
- Verified that existing tests now pass with the new model configurations.
