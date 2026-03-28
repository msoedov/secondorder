# Security Audit Report - TLO-6

Date: 2026-03-28

## Scope

Full codebase scan for hardcoded secrets, API keys, tokens, passwords, and sensitive data.

## Findings

### No hardcoded secrets found

Scanned all Go files, config files, YAML workflows, and static assets. No hardcoded credentials detected.

### Environment variables used

All sensitive values are properly loaded from environment:

| Var | File | Purpose |
|-----|------|---------|
| `TLO_TELEGRAM_TOKEN` | `cmd/thelastorg/main.go:80` | Telegram bot token (optional) |
| `TLO_TELEGRAM_CHAT_ID` | `cmd/thelastorg/main.go:81` | Telegram chat ID (optional) |

### GitHub Actions

`release.yml` uses only `${{ secrets.GITHUB_TOKEN }}` (GitHub-managed, never exposed).

## Changes Made

1. **`.gitignore`** — added exclusions for `.env`, `.env.local`, `.env.*.local`, `*.pem`, `*.key`, `*.p12`, `*.pfx`
2. **`.env.example`** — created with documentation of required env vars (no real values)

## Verification

- `tlo.db` is excluded from git (already in `.gitignore`)
- Binary `thelastorg` is excluded from git
- No private key or certificate files found in the repo
- No base64-encoded secrets or hex strings matching known secret patterns found
