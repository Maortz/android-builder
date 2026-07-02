---
name: merge-verdict
description: >
  Use when checking which open PRs in the android-builder repo are ready to
  merge. Triggers on /merge-verdict, "which PRs are ready to merge", "check
  merge readiness", "give merge verdict", or when asked to audit open PRs for
  merge eligibility. First audits the review gate (reviewed + threads resolved)
  for every PR; then, for review-passing PRs lacking a green CI run, pushes an
  empty commit to re-trigger CI. Never merges or approves.
---

# Merge Verdict

## Overview

Two phases:

1. **Audit the review gate** for every open PR (read-only).
2. **Re-trigger CI** for PRs that pass review but have no current green CI run.
   This pushes an empty commit to each affected PR branch to fire a new run.

Never merge, approve, or request-changes. The only writes this skill performs
are: verdict comments on PRs and empty commits on PR branches.

## Gates

A PR passes the **review gate** when BOTH:

| Gate | Check |
|------|-------|
| **Reviewed** | At least one completed review exists (not just comments) |
| **Comments clean** | No unresolved **blocking** review threads — use GraphQL `reviewThreads { isResolved, comments }`, not REST (REST misses resolution state) |

**Blocking thread definition (shared across skills).** A review thread is
**blocking** when it is unresolved AND its first comment's body begins with
`🔴` (blocker), `🟠` (major), `🟡` (minor), or `🟢` (nit). ALL unresolved
severity threads must be resolved before merge. Only `ported to #N` spinoff
notes and "this revision is clean" confirmations are **non-blocking**. This
matches the severity prefixes the review-orchestrator posts (`🔴 blocker · 🟠 major · 🟡 minor · 🟢 nit`).

A PR also passes the **conflict gate** only when it has **no merge conflicts** —
GitHub's `mergeable` field is `MERGEABLE`, not `CONFLICTING`. A `CONFLICTING` PR
cannot be merged and re-triggering CI on it is pointless: it must be rebased /
have main merged in first (the review-response loop does that, not this skill).
`mergeable` may be `UNKNOWN` while GitHub is still computing it — treat `UNKNOWN`
as "not yet a CI candidate this pass", post an `⏳` verdict (see Step 5), and
re-check next cycle.

A PR is **READY TO MERGE** when it passes the review gate AND the conflict gate
AND has a green CI run on its current head SHA. CI is **green** when every
required check (`build`, `test`) is `SUCCESS`/`NEUTRAL` on the current
`headRefOid`. A PR with no run, a stale run (run SHA ≠ current head), a failed
run, or a pending run is **not green** and (if it also passes review + conflict
gates) is a candidate for re-trigger.

## The CI workflow

- Single workflow: **CI** (`.github/workflows/ci.yml`), always enabled (repo is public, GitHub-hosted runners).
- Triggers on `pull_request`/`push` to `main` — no `workflow_dispatch`. The only way to fire it for a PR is a new commit on the PR branch (`synchronize` event), which is why Phase C pushes an empty commit.

## Steps

### 1 — List open PRs

```
gh pr list --repo Maortz/android-builder --state open --draft=false \
  --json number,title,url,headRefName,headRefOid,reviewDecision,statusCheckRollup,mergeable
```

### 2 — Phase A: evaluate the review gate per PR

```bash
# Review decision
gh pr view <n> --json reviews,reviewDecision

# Unresolved threads — REST misses isResolved; use GraphQL.
# Fetch ALL comments per thread so replies/reasoning can be read:
gh api graphql -f query='
  query($owner:String!,$repo:String!,$number:Int!){
    repository(owner:$owner,name:$repo){
      pullRequest(number:$number){
        reviewThreads(first:50){
          nodes{ isResolved comments(first:10){ nodes{ body author { login } } } }
        }
      }
    }
  }' -f owner=Maortz -f repo=android-builder -F number=<n>

# Also fetch general (issue-level) PR comments — these carry prior verdicts,
# maintainer hold notes, and agent round-summaries:
gh api repos/Maortz/android-builder/issues/<n>/comments
```

A thread counts as **blocking** when `isResolved == false` AND its first
comment's body starts with `🔴`, `🟠`, `🟡`, or `🟢` (see the shared definition above).

When reading a 🟠/🔴 unresolved thread, also read subsequent replies in that
thread: if the agent has replied with a clear technical justification for not
making the change (e.g. "column does not exist in schema, ported to #N"),
treat the thread as **address-responded** and note it in the verdict as
"pending reviewer resolution" rather than "unaddressed feedback" — the PR
still does not pass the review gate until the reviewer resolves the thread,
but the verdict message should explain the state accurately.

**Passes review gate** when:
- `reviewDecision` is `APPROVED`, or at least one review with state `APPROVED`
  or `COMMENTED` exists (i.e. not zero reviews), AND
- Zero unresolved **blocking** `reviewThreads` nodes (only `ported to #N` / "clean"
  threads are ignored; unresolved 🟢/🟡 threads ARE blocking).

**Skip `needs-human-decision` PRs:** if a PR carries the `needs-human-decision` label, skip it entirely — do not audit, do not post a verdict, do not re-trigger CI.

**Explicit hold via general comment:** if a general comment from the maintainer
(author = `Maortz`) contains the phrase `hold:` or `do not merge`, treat the PR
as NOT READY regardless of other gates and include the hold reason in the verdict.

PRs that fail the review gate are **NOT READY** — record the failing reason and
do nothing else to them. They are never candidates for CI re-trigger.

### 3 — Phase B: find review-passers with no green CI

First apply the **conflict gate**. Read `mergeable` from step 1 (or
`gh pr view <n> --json mergeable`):
- `CONFLICTING` → **NOT READY (conflicts)**. Record it and do nothing else — it
  is never a CI re-trigger candidate (CI can't help a conflicted branch; it needs
  main merged in first). Re-triggering would also waste an empty commit.
- `UNKNOWN` → GitHub is still computing mergeability. Skip as a candidate this
  pass and re-check next cycle.
- `MERGEABLE` → eligible to continue.

For each PR that passes BOTH the review gate and the conflict gate, check CI on
the current head SHA:

```bash
gh pr checks <n>   # or read statusCheckRollup from step 1
```

A PR is a **re-trigger candidate** when it passed the review gate AND the
conflict gate (`mergeable == MERGEABLE`) but its required checks are not all
`SUCCESS`/`NEUTRAL` on the current `headRefOid` (no run, stale, failed, or pending).

If there are **zero** candidates, skip Phase C entirely — do not touch the
workflow.

### 4 — Phase C: re-trigger CI

Only run this when Phase B found ≥1 candidate.

```bash
# For each candidate PR, push an empty commit via the Git refs API
# (no local checkout / working-tree churn). Same tree → empty commit.
SHA=$(gh pr view <n> --repo Maortz/android-builder --json headRefOid -q .headRefOid)
BRANCH=$(gh pr view <n> --repo Maortz/android-builder --json headRefName -q .headRefName)
TREE=$(gh api repos/Maortz/android-builder/commits/$SHA --jq .commit.tree.sha)
NEW=$(gh api repos/Maortz/android-builder/git/commits \
        -f message="ci: re-trigger checks" \
        -f tree=$TREE -f parents[]=$SHA --jq .sha)
gh api -X PATCH repos/Maortz/android-builder/git/refs/heads/$BRANCH -f sha=$NEW
```

Do **not** wait for runs to finish. Their green/red result is picked up by a later merge-verdict run.

### 5 — Post a verdict comment on each PR

```
gh pr comment <n> --repo Maortz/android-builder --body "<verdict>"
```

Anti-spam: before posting, check the PR's most recent comment — if it already
carries an identical verdict, skip the post.

Verdict bodies:
- `✅ **READY TO MERGE** — reviewed, all threads resolved, no conflicts, CI green.`
- `🔄 **CI RE-TRIGGERED** — review passed, no conflicts; CI was missing/stale/failed, fresh run queued. Not ready until it goes green.`
- `⏳ **NOT READY (mergeability unknown)** — review passed and CI is green on head \`{sha}\`, but GitHub has not yet computed mergeability. Re-check next cycle.`
- `❌ **NOT READY (conflicts)** — branch has merge conflicts with main; rebase or merge main in before CI can run / it can merge.`
- `❌ **NOT READY (review)** — <reason: "no review yet" / "<N> unresolved blocking thread(s): <brief description — include whether agent replied with reasoning or thread is fully unaddressed>">`
- `❌ **NOT READY (hold)** — maintainer hold: <quote the hold comment>.`

### 6 — Print summary

```
PR    | Title                  | Verdict
------|------------------------|--------
#42   | feat: add X            | ✅ READY (review + no conflicts + CI green)
#39   | feat: add Z            | 🔄 CI re-triggered (pending)
#38   | fix: Y                 | ❌ review: 2 unresolved threads
#36   | feat: add V            | ❌ conflicts: needs rebase
#35   | chore: W               | ❌ review: no review yet
```

Verdict values:
- `✅ READY` — passed review + conflict gates and CI already green on head SHA.
- `🔄 CI re-triggered (pending)` — passed review + conflict gates, CI was
  missing/stale/failed, a fresh run was queued this run.
- `⏳ mergeability unknown` — review passed, CI green, but `mergeable == UNKNOWN`; re-check next cycle.
- `❌ conflicts: needs rebase` — `mergeable == CONFLICTING`; needs main merged in
  before CI runs or it can merge (handled by the review-response loop, not here).
- `❌ review: <reason>` — failed the review gate (one or more unresolved 🔴/🟠/🟡/🟢 threads, or no review yet).
- `❌ hold: <reason>` — explicit maintainer hold comment.

## Constraints

- Never merge, approve, request-changes, or edit application code.
- The only writes allowed: verdict comments on PRs and empty commits on candidate
  PR branches. Nothing else.
- Pushing an empty commit advances `headRefOid`; if branch protection has
  "dismiss stale reviews" enabled this re-opens the review gate next run. That
  is acceptable — the PR simply isn't READY until re-reviewed.
