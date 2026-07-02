---
name: agent-pipeline
description: >
  Use when running the full autonomous development pipeline on the android-builder
  repo in an endless loop: implement issues, review PRs, address review comments,
  then re-trigger CI for merge-ready PRs — in that order, each drained to
  completion before moving to the next. Triggers on
  /agent-pipeline, "run full pipeline", "run all orchestrators", or when invoked
  from the cron watchdog. Loops forever with backoff between full cycles.
---

# Agent Pipeline

## Overview

Endless pipeline loop. Each stage runs until it has nothing left to do, then the next stage starts. After all four stages are drained, wait (backoff) and restart from stage 1.

**Stage order:** implement → review → address comments → merge-verdict

Runs as a persistent window in the tmux `claude` session. Cron is a watchdog only — if the window dies, cron recreates it within the hour. `flock` prevents duplicate instances.

---

## Pipeline Loop

```
loop forever:
  stage 1: drain issue-implementer
  stage 2: drain review-orchestrator
  stage 3: drain review-response-orchestrator
  stage 4: merge-verdict (audit review gate + re-trigger CI for review-passing PRs)
  backoff (default: 30 min)
```

### Stage 1 — Implement issues

Dispatch a **fresh general-purpose (opus) agent** with this brief:
> You are running the `issue-implementer` skill in Maortz/android-builder. Read `.claude/skills/issue-implementer/SKILL.md`. Execute it in **single-pass mode**: run the orchestrator loop (O1→O4) picking and implementing issues one at a time until O2 finds nothing qualifying. Then STOP — do NOT loop back. The outer pipeline handles cycling. Return exactly: `DONE` (nothing left), `STOPPED <reason>`, or `FAILED <reason>` as your last line.

On `FAILED`: log, proceed to stage 2.

### Stage 2 — Review PRs

Dispatch a **fresh general-purpose (sonnet) agent** with this brief:
> You are running the `review-orchestrator` skill in Maortz/android-builder. Read `.claude/skills/review-orchestrator/SKILL.md`. Execute it in **single-pass mode**: dispatch reviewer agents for every currently-open PR that lacks an up-to-date `<!-- staff-review:<HEAD_SHA> -->` marker. Once all open PRs are covered, STOP — do NOT loop back to check for new PRs. The outer pipeline handles cycling. Return `DONE`, `STOPPED <reason>`, or `FAILED <reason>` as your last line.

On `FAILED` twice in a row: log, proceed to stage 3.

### Stage 3 — Address review comments

Dispatch a **fresh general-purpose (opus) agent** with this brief:
> You are running the `review-response-orchestrator` skill in Maortz/android-builder. Read `.claude/skills/review-response-orchestrator/SKILL.md`. Execute it in **single-pass mode**: run the orchestrator loop (O1→O4) walking the whole open-PR backlog from the lowest number upward — a per-PR `BLOCKED_NEEDS_DECISION`/`STOPPED`/`FAILED` skips that PR and continues to the next; only a global fault halts. Continue until O2 finds nothing qualifying. Then STOP — do NOT loop back. The outer pipeline handles cycling. End your output with the **blocked report** (every PR returning `BLOCKED_NEEDS_DECISION` + reason), then `DONE` (nothing left), `STOPPED <reason>`, or `FAILED <reason>` as your last line.

On `FAILED`: log, continue to stage 4. **Surface the blocked report** (PRs labeled `needs-human-decision`) to the maintainer in the cycle summary.

### Stage 4 — Merge verdict (CI re-trigger)

Dispatch a **fresh general-purpose (sonnet) agent** with this brief:
> You are running the `merge-verdict` skill in Maortz/android-builder. Read `.claude/skills/merge-verdict/SKILL.md`. Execute it **once** over all currently-open non-draft PRs: audit the review gate, then for review-passing PRs lacking a green CI run on their current head, enable the `CI` workflow, push an empty commit to re-trigger it, wait until each run is queued, then disable the workflow again. Post verdict comments. NEVER merge, approve, or request-changes. This is a single pass — do NOT loop. The outer pipeline handles cycling. Return exactly: `DONE`, `STOPPED <reason>`, or `FAILED <reason>` as your last line.

On `FAILED`: log, continue to backoff. Re-triggered CI runs finish during the backoff window, so the next cycle's stage 2/4 see fresh check results.

### Backoff + pause check

After all four stages complete (or are skipped): **sleep 60 minutes**.

Then, before starting the next cycle, check the pause gate:
- If `/tmp/cron-paused` exists → keep sleeping in 5-minute increments, printing "paused — waiting..." each time, until the file is removed.
- Once clear → proceed to stage 1.

Rationale: gives CI time to run on newly-pushed branches, GitHub to process new review comments, and avoids hammering the API in a tight loop when there genuinely is no work. Pause gate allows interrupting between cycles without killing the tmux window.

---

## Failure handling

| Situation | Action |
|-----------|--------|
| Stage 1 FAILED | Log, continue to stage 2 |
| Stage 2 FAILED ×2 consecutive | Log, continue to stage 3 |
| Stage 3 FAILED | Log, continue to stage 4 |
| Stage 4 FAILED | Log, continue to backoff |
| Any stage STOPPED | Normal — move to next stage |
| 5+ consecutive full-cycle failures | STOP and report — something systemic is broken |
