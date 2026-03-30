# Decision: SO_API_KEY missing from auditor environment

Date: 2026-03-29
Audit runs affected: 46abf99d, a4895fbc, 65ff2dbb, 05f3fb5b (4 consecutive)

---

## Problem

The auditor agent is invoked without SO_API_KEY in its environment. Every audit run since the first has produced valid archetype patches and feature requests that could not be submitted. The patches accumulate in artifact-docs/decisions/ as dead documentation.

## Impact

- 4 audit runs × ~16 issues worth of analysis → zero archetype improvements applied
- Founding Engineer, CEO, QA, DevOps archetypes remain at initial versions
- Chrome MCP requirement exists only in policy, not in QA archetype
- XSS security guidance exists only in policy, not in Founding Engineer archetype

## Recommendation

The operator should do ONE of the following:
1. Set SO_API_KEY as an environment variable in the audit agent's invocation config
2. Apply the patches from decisions/archetype-patches-65ff2dbb.md manually via the settings UI

## Patches ready to apply

See: `decisions/archetype-patches-65ff2dbb.md`

Agents to patch: CEO, founding-engineer, qa-engineer, devops
