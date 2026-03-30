# Contributing to secondorder

## Secret Scanning

### What and Why

We use [gitleaks](https://github.com/gitleaks/gitleaks) to prevent secrets (API keys, passwords, tokens) from being committed to the repository. This runs as a pre-commit check and in CI to catch accidental credential leaks before they reach the remote.

Why it matters:
- secondorder handles API keys, encrypted secrets, and agent credentials
- A leaked secret in git history is effectively permanent (force-pushing doesn't remove it from clones)
- Automated scanning catches what code review misses

### Local Setup

**Install gitleaks:**

```bash
# macOS
brew install gitleaks

# Go install
go install github.com/zricethezav/gitleaks/v8@latest

# Or download a binary from https://github.com/gitleaks/gitleaks/releases
```

**Run a scan manually:**

```bash
# Scan the entire repo
gitleaks detect --source . -v

# Scan only staged changes (useful before committing)
gitleaks protect --staged -v
```

**Set up as a pre-commit hook (recommended):**

```bash
# Create or edit .git/hooks/pre-commit
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/sh
gitleaks protect --staged -v
EOF
chmod +x .git/hooks/pre-commit
```

After this, every `git commit` will automatically scan staged files for secrets.

### Configuration

The project includes a `.gitleaks.toml` configuration file at the repository root. This file:
- Extends the default gitleaks ruleset
- Adds custom rules for secondorder-specific patterns (`so_` API keys, Telegram bot tokens)
- Allowlists test files (`_test.go`, `testdata/`, `fixtures/`) and known test placeholder values
- Allowlists `go.sum` hashes which are not secrets

You can also use `make scan` to run a full repo scan.

### CI Integration

Gitleaks runs automatically on every push and pull request via GitHub Actions. The workflow is defined in `.github/workflows/ci.yml`. If the scan finds a potential secret, the CI check will fail and block the PR.

### Handling False Positives

If gitleaks flags something that is not a real secret (e.g., a test fixture, a hash constant, example placeholder), you have two options:

**Option 1: Inline allowlist comment**

Add a `gitleaks:allow` comment on the line:

```go
testAPIKey := "test-key-not-real-abc123" // gitleaks:allow
```

**Option 2: Update `.gitleaks.toml`**

Add the path or pattern to the allowlist in `.gitleaks.toml`:

```toml
[allowlist]
  paths = [
    '''testdata/.*''',
  ]
```

When adding allowlist entries:
1. Verify the flagged value is genuinely not a secret
2. Prefer the narrowest possible allowlist rule (inline comment > specific path > broad pattern)
3. Add a comment in `.gitleaks.toml` explaining why the entry is allowed

### What To Do If You Accidentally Commit a Secret

1. **Do not push.** If you haven't pushed yet, amend the commit to remove the secret.
2. **If already pushed:** Rotate the secret immediately. Treat it as compromised regardless of how quickly you act.
3. Contact the team so the old credential can be revoked.
4. Use `git filter-repo` or BFG Repo-Cleaner to remove the secret from history if needed.

### Tool Documentation

- Gitleaks GitHub: https://github.com/gitleaks/gitleaks
- Gitleaks configuration reference: https://github.com/gitleaks/gitleaks#configuration
- OWASP secrets management cheat sheet: https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html
