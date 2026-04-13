# Agent Feature Flags

Agent settings include boolean feature flags that control Claude runner behavior.

## disableSkills

When `true`, the agent runner is launched with `--disable-slash-commands`, which prevents
Claude from loading or executing skills (slash commands).

Default: `false` (skills enabled).

Use when you want a focused agent that cannot invoke skills at runtime — reduces surface area
for unexpected behavior in production or sandboxed agents.

## disableSlashCommands

When `true`, also passes `--disable-slash-commands` to the runner.

Both `disableSkills` and `disableSlashCommands` map to the same Claude CLI flag. Either one
being `true` is sufficient to disable slash commands.

## disallowedTools

A list of tool names that Claude is not permitted to use. Each entry becomes a separate
`--disallowedTools <name>` argument. Empty strings in the list are ignored.

## DB columns

```
agents.disable_slash_commands  INTEGER NOT NULL DEFAULT 0
agents.disable_skills          INTEGER NOT NULL DEFAULT 0
agents.disallowed_tools        TEXT (comma-separated)
```

Migration: `025_agent_disable_flags.sql`, `026_agent_disallowed_tools.sql`.
