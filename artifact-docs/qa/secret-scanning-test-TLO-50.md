# Secret Scanning Test Results (TLO-50)

Date: 2026-03-29
Tool: gitleaks 8.30.1
Config: `.gitleaks.toml`

## Test execution

```
bash scripts/test-secret-scanning.sh
```

Results: **11 passed, 0 failed**

## Positive detection cases (must be caught)

| # | Pattern | Rule triggered | Result |
|---|---------|---------------|--------|
| 1 | TLO API key (`tlo_supersecretkey12345678`) in `.go` source | `tlo-api-key` | PASS |
| 2 | Telegram bot token (`123456789:AAHFGqk...`) in shell script | `telegram-bot-token` | PASS |
| 3 | Stripe live key (`sk_live_...`) in `.go` source | `stripe-access-token` | PASS |
| 4 | AWS-format key (`AKIAI1234567890ABCDE`) in `.go` source | `generic-api-key` | PASS |

## Negative cases (must NOT be flagged — false positive prevention)

| # | Pattern | Allowlist mechanism | Result |
|---|---------|-------------------|--------|
| 5 | `tlo_test_key_123` in non-test file | regex allowlist | PASS |
| 6 | TLO key in `*_test.go` file | path allowlist | PASS |
| 7 | TLO key in `testdata/` directory | path allowlist | PASS |
| 8 | `$THELASTORG_API_KEY` env var placeholder | regex allowlist | PASS |
| 9 | `tlo_abcdef1234567890` documentation example | regex allowlist | PASS |
| 10 | `go.sum` hash lines | regex allowlist | PASS |

## Repo baseline

| Check | Result |
|-------|--------|
| Working tree clean (no-git scan) | PASS |

## Issues found and fixed

### False positive in docs
`artifact-docs/infra/secret-scanning-TLO-49.md:15` contained `tlo_abcdef1234567890` as a
documentation example. The scanner flagged it. Fixed by adding the exact string to the
regex allowlist in `.gitleaks.toml`.

### Test script path allowlist
`scripts/test-secret-scanning.sh` embeds intentional fake secrets as heredoc test fixtures.
Added to path allowlist so the test script itself does not trigger the scanner.

### Invalid `--quiet` flag
gitleaks 8.30.1 does not support `--quiet`. Fixed by using `--no-banner -l error` instead.

## CI integration

The `secret-scan` job in `.github/workflows/ci.yml` runs `gitleaks/gitleaks-action@v2`
with `fetch-depth: 0` (full history) on every push and PR. It uses `.gitleaks.toml`
automatically (action reads it from the repo root).

## Manual checklist

1. Push a branch containing `tlo_realkey1234567890abc` in a `.go` file — CI should fail
2. Remove the secret and repush — CI should pass
3. Verify `*_test.go` files with fake keys do not fail CI
4. Verify `testdata/` files with fake keys do not fail CI
5. Check `gitleaks/gitleaks-action` version pins are up to date quarterly
