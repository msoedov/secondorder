---
issue: SO-58
title: CLI: Add template (-t) and model (-m) flags for agent runs
---

## Overview

The `secondorder` binary accepts two new CLI flags for controlling startup template and default agent runtime.

## Flags

| Flag | Alias | Env | Default | Description |
|------|-------|-----|---------|-------------|
| `--template` | `-t` | `TEMPLATE` | `startup` | Team template: startup, dev-team, enterprise, saas, agency |
| `--model` | `-m` | `MODEL` | `` (empty) | Default agent runner: `claude`, `gemini`, `codex` |

## Usage

```
secondorder [-t <template>] [-m <model>] [port]
```

Examples:

```sh
# Use startup template with gemini as the default runner
secondorder -t startup -m gemini

# Use dev-team template with codex runner on port 8080
secondorder -t dev-team -m codex 8080

# Use environment variables
TEMPLATE=saas MODEL=claude secondorder
```

## Model values

| Flag value | Runner stored on agent |
|------------|----------------------|
| `claude`   | `claude_code`        |
| `gemini`   | `gemini`             |
| `codex`    | `codex`              |

The runner is applied to all agents created during initial template setup. If the database already has agents, the flag has no effect (template is only applied on first run).
