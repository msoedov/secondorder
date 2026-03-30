---
name: Security First
description: Security vulnerabilities must be fixed before any other work proceeds
type: project
---

P0 security findings halt all non-security work in the current block. The founding engineer owns the fix; QA verifies before other issues resume.

**Why:** SO-14 (XSS vulnerabilities) required 4 runs — the highest retry count across both shipped blocks. No archetype guidance existed on HTML template safety, leading to repeated partial fixes.

**How to apply:** When QA flags a P0 security issue, CEO must pause the block, assign a dedicated fix issue to founding-engineer, and not mark the block done until QA verifies the fix.
