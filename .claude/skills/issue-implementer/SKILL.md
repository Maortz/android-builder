---
name: issue-implementer
description: >
  Use when running as a scheduled autonomous agent on the android-builder repo
  (Maortz/android-builder) to pick and claim one GitHub issue labeled agent-ready,
  then dispatch an implementation agent for it. Safe to run in parallel — uses
  claim-issue to prevent two agents grabbing the same issue. Triggers on /orchestrate,
  "run orchestrator", "pick next issue", "dispatch agent for issue", or when invoked
  unattended on a schedule to drive the implementation backlog forward.
---

# Autonomous Issue Orchestrator

## Overview

You are the Orchestrator. You do NOT write code yourself. You pick one `agent-ready` issue and dispatch two sequential agents for it (planning then execution), then act on the result. Both agents are dispatched directly by you — never nested inside each other.

## Orchestrator Loop

### O0 — Tooling (once, before the loop)

```
gh auth status
go version
```

- `gh` not authenticated → **STOP**
- `go` unavailable → only dispatch `area:infra` doc-only issues; none qualify → **STOP**

### O1 — Clean slate (once per pass, before the first O2)

```
git status
git fetch origin && git checkout main && git pull --ff-only
git log origin/main..HEAD
```

- Dirty working tree → **STOP and report**
- Unexpected local commits found → **STOP and report** (don't build on someone else's work)

Run O1 only once at the start of the pass. After each O4 loop-back, skip back directly to O2 — no need to re-run O1 (the impl agent leaves main clean on any exit).

### O2 — Claim work

Maintain an `attempted` set (issue numbers already tried this pass — PRs opened, stopped, or failed).

Use the **claim-issue** skill to pick and atomically label one issue. Pass the `attempted` set so the skill skips already-tried issues.

- Skill returns `CLAIMED <N> <url>` → proceed with that issue number N
- Skill returns `NONE` → **STOP** (nothing left this pass)

### O3 — Two sequential agents per issue (both dispatched by the orchestrator)

The orchestrator dispatches two agents in sequence — never in parallel, never nested. Agent A plans; agent B executes. The orchestrator waits for A to finish before dispatching B.

#### O3a — Planning agent

Spawn a **general-purpose (opus)** agent with the **Planning Agent Brief** below, passing issue number N. Wait for completion. The planning agent does NOT spawn further agents.

| Return | Action |
|--------|--------|
| `PLAN_READY <branch> <plan-path>` | Proceed to O3b with the branch name and plan path |
| `STOPPED <reason>` | Log; release claim; add N to `attempted`; go back to O2 |
| `FAILED <reason>` | Log; release claim; add N to `attempted`; increment fail counter; if ≥ 3 → **STOP**; else go back to O2 |

#### O3b — Execution agent

Spawn a **general-purpose (opus)** agent with the **Execution Agent Brief** below, passing issue number N, the branch name, and the plan path returned by O3a. Wait for completion. The execution agent does NOT spawn further agents.

### O4 — Act on execution return contract

| Return | Action |
|--------|--------|
| `PR_OPENED <url>` | Add N to `attempted`; go back to O2, pick next issue, loop |
| `STOPPED <reason>` | Log reason; add N to `attempted`; go back to O2, pick next issue, loop |
| `FAILED <reason>` | Log reason; add N to `attempted`; increment consecutive-fail counter; if counter ≥ 3 → **STOP** (systemic fault); else go back to O2, pick next issue, loop |

A single issue being stopped or failing is **not** a reason to halt — only O2 returning `NONE` or 3 consecutive `FAILED` results signals a global halt.

Never merge a PR. Never force-push. Never remove the `agent-ready` label gate.

---

## Planning Agent Brief (dispatch to fresh opus agent for issue N)

> You are a Senior Go Engineer planning the implementation of exactly one issue, **#N**, in Maortz/android-builder — a Go CLI (Cobra-based) that orchestrates remote Android APK builds via GitHub Actions and hot-reload dev sessions via `flutter attach`. You do NOT write code yet — you produce a detailed implementation plan that a separate execution agent will follow exactly.

### A1 — Context

Read:
- `CLAUDE.md` — architecture, key files, build/test commands and operational gotchas

### A2 — Understand

```
gh issue view N
```

Read the referenced code paths (see `CLAUDE.md`'s Key Files table for the relevant package).

Scope ambiguous or clearly larger than the effort label implies → comment on the issue explaining why → release claim:
```
gh issue edit N --repo Maortz/android-builder --remove-label agent-in-progress
```
→ return `STOPPED <reason>`. Do not guess.

### A3 — Branch

```
git fetch origin && git checkout main && git pull --ff-only
git checkout -b agent/issue-N-<short-slug>
```

### A4 — Write plan

Use the **superpowers:writing-plans** skill to produce a complete implementation plan. The plan must:

- Follow the writing-plans skill exactly (header, file structure, bite-sized TDD tasks with real code, no placeholders)
- Cover: branch is already created (A3 done), execution starts at the first code task
- Include all verify steps: `go build -o builder ./cmd/builder`, `go vet ./...` (clean), `go test ./...` (all green) — one command at a time, no `&&` chaining
- Include A7 drift check: `git fetch origin && git log origin/main..HEAD --oneline` — STOPPED if foreign commits visible
- Include A8 commit + PR (body: `Closes #N`, change summary, vet/test/build results; commit footer: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`)
- Include A9: comment on issue N linking PR; release claim: `gh issue edit N --repo Maortz/android-builder --remove-label agent-in-progress`
- Note staff-level standards throughout: idiomatic Go (gofmt-clean, errors wrapped with `%w`, no naked panics in library code), package boundaries respected (`internal/` packages stay internal), Cobra command conventions matching existing `cmd/builder/*.go` files

Save to: `docs/superpowers/plans/YYYY-MM-DD-issue-N-<short-slug>.md`

Cannot produce a complete, non-placeholder plan → release claim → return `FAILED <reason>`.

### Planning Agent Return Contract

Last line must be **exactly one** of:

```
PLAN_READY agent/issue-N-<short-slug> docs/superpowers/plans/YYYY-MM-DD-issue-N-<short-slug>.md
STOPPED <reason>
FAILED <reason>
```

---

## Execution Agent Brief (dispatch to fresh opus agent for issue N, branch B, plan P)

> You are a Senior Go Engineer executing an implementation plan for issue **#N** in Maortz/android-builder. The planning agent has already created branch **B** and saved the plan at **P**. Execute the plan task-by-task inline — do NOT spawn further sub-agents.

### E1 — Check out branch

```
git fetch origin && git checkout B
```

Verify the plan file exists at path P. If branch or plan missing → release claim:
```
gh issue edit N --repo Maortz/android-builder --remove-label agent-in-progress
```
→ return `FAILED branch or plan not found`.

### E2 — Execute plan

Read plan P. Execute all tasks task-by-task inline in this session. Follow the plan exactly — no improvisation, no scope creep. Do not spawn sub-agents.

If you hit a blocker that cannot be resolved without human input → release claim → return `STOPPED <reason>`.

If verify (vet/test) cannot be made green after exhausting plan steps → comment on issue N with failing output → release claim → return `FAILED <reason>`.

**Release claim on any early exit:**
```
gh issue edit N --repo Maortz/android-builder --remove-label agent-in-progress
```

### E3 — After plan execution completes

The plan covers all remaining steps (A7 drift check, A8 commit+PR, A9 comment+release). Confirm each was executed. If the plan omitted any step, execute it now:

- **A7**: `git fetch origin && git log origin/main..HEAD --oneline` — foreign commits → `STOPPED <reason>`.
- **A8**: Push branch, `gh pr create --base main`. PR body: `Closes #N`, change summary, vet/test/build results. Commit footer: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- **A9**: Comment on issue N linking PR. Release claim: `gh issue edit N --repo Maortz/android-builder --remove-label agent-in-progress`.

### Execution Agent Return Contract

Last line must be **exactly one** of:

```
PR_OPENED <url>
STOPPED <reason>
FAILED <reason>
```

Never merge. Never force-push. Never touch an issue lacking the `agent-ready` label.
