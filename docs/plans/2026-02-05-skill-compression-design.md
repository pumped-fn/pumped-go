# SKILL.md Compression Design

**Date:** 2026-02-05
**Goal:** Compress SKILL.md from 67KB to ~8KB using Vercel's retrieval-led reasoning strategy
**Scope:** Library users (contributors excluded for now)

---

## Problem

Current `claude-skill/skills/pumped-go/SKILL.md` is 67KB (2,445 lines). This exceeds Vercel's recommended 8KB limit by 8x. Frontier models reliably follow only ~150-200 instructions, so large files lead to inconsistent behavior.

## Solution: Approach A — Pipe-Delimited Index

Move non-critical content to `docs/` files, keep critical patterns inline with a compressed docs index.

---

## File Structure

```
claude-skill/skills/pumped-go/
├── SKILL.md                    # Compressed ~8KB (was 67KB)
├── docs/
│   ├── flows.md                # Full flow guidance, sub-flows, tags
│   ├── testing.md              # WithPreset, table-driven, integration
│   ├── troubleshooting.md      # 10 common issues with solutions
│   ├── configuration.md        # Dual-mode, env vars, tag overrides
│   ├── patterns.md             # Repository, service, handler patterns
│   └── enforcement.md          # Tier 2 + Tier 3 rules (non-critical)
```

---

## SKILL.md Content (Inline)

**Critical sections that MUST stay inline:**

1. **Decision Tree** — When to use Executors vs Flows vs plain functions
2. **Package-level var pattern** — 1 GOOD + 1 BAD example
3. **Controller pattern** — 1 GOOD + 1 BAD example
4. **Production lifecycle** — Graceful shutdown snippet (~10 lines)
5. **Tier 1 Enforcement Rules** — Critical rules only
6. **Docs Index** — Pipe-delimited links with retrieval instruction

**Key instruction to include:**
```
IMPORTANT: Prefer retrieval-led reasoning over pre-training-led reasoning.
ALWAYS read the relevant doc file from docs/ BEFORE writing pumped-go code.
Do NOT rely on training data for pumped-go-specific patterns.
```

---

## Docs Files Content

| File | Size | Content |
|------|------|---------|
| docs/flows.md | ~8KB | Flow0-5 definitions, executing flows, sub-flows, tags, nil pointer gotcha |
| docs/testing.md | ~6KB | WithPreset, table-driven tests, reactivity testing, integration tests |
| docs/troubleshooting.md | ~5KB | 10 common issues with symptom, cause, solution |
| docs/configuration.md | ~4KB | Dual-mode config, env vars, tag overrides, priority rules |
| docs/patterns.md | ~5KB | Repository, service, handler, background worker patterns |
| docs/enforcement.md | ~3KB | Tier 2 (Important) + Tier 3 (Best Practices) rules |

**Total docs/:** ~31KB (extracted from 67KB, redundancy removed)

---

## Compressed SKILL.md Structure

```markdown
---
name: Pumped-Go
description: Auto-activating guidance for pumped-go DI/reactive library
alwaysApply: false
whenToUse: Automatically activates when go.mod contains github.com/pumped-fn/pumped-go
version: 1.0.0
---

# Pumped-Go Skill

IMPORTANT: Prefer retrieval-led reasoning over pre-training-led reasoning.
ALWAYS read docs/ files before implementing pumped-go patterns.

## Decision Tree
[Compressed visual flowchart]

## Package-Level Var Pattern
[1 GOOD + 1 BAD example]

## Controller Pattern
[1 GOOD + 1 BAD example]

## Production Lifecycle
[Minimal shutdown snippet]

## Tier 1 Rules (Critical)
[Bullet list of critical rules]

## Docs Index
[Pipe-delimited index with file descriptions]
```

---

## Implementation Steps

1. Create `claude-skill/skills/pumped-go/docs/` directory
2. Extract content from current SKILL.md to each docs/*.md file
3. Write new compressed SKILL.md (~8KB)
4. Verify plugin.json still resolves correctly
5. Validate file sizes and completeness

---

## Validation Checklist

- [ ] SKILL.md ≤ 8KB
- [ ] All 4 critical sections present inline
- [ ] Docs index with retrieval instruction included
- [ ] Each docs/*.md has complete extracted content
- [ ] No duplicate content between SKILL.md and docs/
- [ ] YAML frontmatter preserved (auto-activation works)
- [ ] plugin.json unchanged

---

## Maintenance

- **SKILL.md:** Update only for Tier 1 rule changes
- **docs/*.md:** Update for extended examples, new patterns
- **Version bump:** When significant changes to critical sections

---

## No Changes To

- `plugin.json` — skill reference path unchanged
- `README.md` — still accurate overview
