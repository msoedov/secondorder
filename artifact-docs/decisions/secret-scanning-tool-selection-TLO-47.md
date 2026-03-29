---
issue: TLO-47
date: 2026-03-29
status: decided
decision: gitleaks
---

# Secret Scanning Tool Selection

## Candidates evaluated

| Tool | Language | License | Last release |
|------|----------|---------|--------------|
| [gitleaks](https://github.com/gitleaks/gitleaks) | Go | MIT | Active (v8.x) |
| [truffleHog v3](https://github.com/trufflesecurity/trufflehog) | Go | AGPL-3.0 | Active (v3.x) |
| [detect-secrets](https://github.com/Yelp/detect-secrets) | Python | Apache-2.0 | Active |
| [git-secrets](https://github.com/awslabs/git-secrets) | Shell | Apache-2.0 | Stale (2021) |

---

## Comparison

### gitleaks

**Pros**
- Written in Go — single static binary, trivial to install in CI
- Official `gitleaks/gitleaks-action@v2` for GitHub Actions
- Scans both working tree and full git history
- Rich TOML config for custom rules and allowlists
- Fast: parallel scanning, incremental with `--log-opts`
- 150+ built-in rules covering AWS, GCP, GitHub, Stripe, etc.
- Pre-commit hook support via `gitleaks protect`
- MIT licensed — no copyleft concerns

**Cons**
- Custom rule authoring requires regex knowledge
- No SaaS dashboard (self-contained tool only)

---

### truffleHog v3

**Pros**
- 700+ detectors with active verification (calls the actual API to confirm validity)
- Entropy-based and regex detection
- GitHub Actions support

**Cons**
- AGPL-3.0 license (copyleft)
- Slower due to live secret verification
- Higher false-negative rate in offline/airgapped environments
- Python dependency for local dev (heavier than a Go binary)

---

### detect-secrets

**Pros**
- Designed for pre-commit integration
- Baseline file model (only alerts on new secrets)
- Apache-2.0 license

**Cons**
- No native GitHub Actions integration (requires wrapper)
- Does not scan full git history out of the box
- Baseline management adds operational overhead
- Python dependency

---

### git-secrets

**Pros**
- Minimal — shell script, no dependencies
- Simple pattern matching

**Cons**
- No git history scanning
- No built-in ruleset; every pattern must be added manually
- Last commit 2021 — effectively unmaintained
- AWS-pattern focused, not general purpose

---

## Decision: gitleaks

**Rationale**

gitleaks is the best fit across all acceptance criteria:

1. **GitHub Actions integration** — official action (`gitleaks/gitleaks-action@v2`), zero config required for basic use
2. **Go codebase support** — language-agnostic scanner; native Go binary means no runtime dependency
3. **Active maintenance** — v8.x series, frequent releases, large community
4. **Permissive license** — MIT, no AGPL concerns for a private codebase

truffleHog's live verification is valuable for incident response but adds latency and AGPL licensing complexity that is not justified for a CI scan. detect-secrets has a good pre-commit story but lacks history scanning. git-secrets is too limited and unmaintained.

## Implementation

See [secret-scanning-TLO-49.md](../infra/secret-scanning-TLO-49.md) for gitleaks configuration and CI integration details.
