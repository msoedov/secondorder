---
name: Bug Fix Protocol
description: Founding Engineer must reproduce a bug before attempting a fix
type: project
---

Before writing any fix, reproduce the bug with a concrete test case or reproduction steps. Do not patch based on reading code alone.

**Why:** SO-43 (6 runs) and SO-36 (4 runs) both involved complex state bugs where code-reading led to incomplete fixes. Reproduction forces the correct failure mode to be visible before the fix is written.

**How to apply:** Founding Engineer must document reproduction steps or a failing test in their first comment on a bug issue. If reproduction fails (bug not observable), escalate to CEO before proceeding.
