# Secret Scanning Configuration (SO-49)

## Tool

[Gitleaks](https://github.com/gitleaks/gitleaks) — scans git history and working tree for secrets.

## Config file

`.gitleaks.toml` in the repo root.

## What is detected

| Rule | Pattern | Example |
|------|---------|---------|
| `so-api-key` | `so_` prefix + 16+ alphanum chars | `so_abcdef1234567890` |
| `telegram-bot-token` | Telegram bot token format | `123456789:ABCdef...` |
| Default ruleset | Generic API keys, AWS, GCP, GitHub tokens, private keys, etc. | (via `useDefault = true`) |

## False positive suppression

The allowlist excludes:
- All `*_test.go` files (test fixtures use fake keys by design)
- `testdata/` and `fixtures/` directories
- Known stub values used in unit tests (`so_test_key_123`, `so_dup_test_key`, `tok123`)
- Environment variable references like `$SECONDORDER_API_KEY`
- `go.sum` hash lines

## Running locally

```sh
# Install
brew install gitleaks

# Scan working tree
gitleaks detect --source . --config .gitleaks.toml

# Scan full git history
gitleaks detect --source . --config .gitleaks.toml --log-opts="--all"
```

## CI integration

Add to the CI pipeline (e.g. GitHub Actions):

```yaml
- name: Secret scan
  uses: gitleaks/gitleaks-action@v2
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  with:
    config-path: .gitleaks.toml
```

## Adding new allowlist entries

Edit `.gitleaks.toml` under `[allowlist].regexes`. Be specific — prefer exact string matches over broad regexes to avoid masking real secrets.
