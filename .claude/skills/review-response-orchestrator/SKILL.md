---
name: review-response-orchestrator
description: >
  Use when running as a scheduled autonomous agent on the android-builder repo
  (Maortz/android-builder) to find open agent-authored PRs with unresolved review
  feedback or merge conflicts and dispatch one agent per PR to address it. Triggers on
  /address-comments, "address review comments", "respond to PR feedback",
  "dispatch comment-response agent", or when invoked unattended to drive
  review-response work. Never merges, never force-pushes shared branches.
---

# Review Response Orchestrator

## Overview

You are the Review Response Orchestrator. You do NOT write code yourself. You find open agent-authored PRs with unresolved review feedback **or merge conflicts** and dispatch one fresh agent per PR to address it. Each PR's context (diff, comments, build output) stays encapsulated in its own agent.

---

## Orchestrator Loop

### O0 — Tooling (once)

```
gh auth status
go version
```

- `gh` not authenticated → **STOP**
- `go` unavailable → **STOP** (cannot satisfy verify gate; must not push code changes)

### O1 — Clean slate

```
git status
git fetch origin && git checkout main && git pull --ff-only
git log origin/main..HEAD
```

- Dirty working tree → **STOP and report**
- Unexpected local commits → **STOP and report** (don't build on top of someone else's work)

### O2 — Pick work

Keep an **`attempted` set** of PR numbers already tried this pass (starts empty).
It prevents re-picking a PR you just skipped/blocked and looping forever.

```
gh pr list --repo Maortz/android-builder --state open \
  --json number,title,url,isDraft,reviewDecision,labels,mergeable
```

**Consider only:** non-draft PRs authored by an agent (branch prefix `agent/`).

**Exclude up front:**
- any PR already in the `attempted` set this pass, and
- any PR carrying the `needs-human-decision` label (a prior run escalated it for
  a maintainer call — see O4 / A2; do not retry until a human removes the label).

For each remaining candidate fetch review threads:
```
gh pr view <n> --json reviews,comments
# + GraphQL reviewThreads { isResolved, comments(first:1){ nodes{ body } } } query
# (REST comments alone miss resolution state)
```

**Blocking thread definition (shared across skills).** A thread is **blocking**
when it is unresolved AND its first comment's body begins with `🔴` (blocker),
`🟠` (major), `🟡` (minor), or `🟢` (nit). ALL severity threads are actionable
review-response work — agents must address them. Only `ported to #N` / "clean"
confirmations are non-blocking and NOT actionable.

**A PR qualifies** if ANY of the following are true:
- It has at least one unresolved **blocking** thread
- It has a `CHANGES_REQUESTED` review decision
- It has merge conflicts with main (check `mergeable` field: `CONFLICTING`)

(Do not pick PRs whose only unresolved threads are ported/clean AND have no conflicts — there is nothing to fix.)

**Skip** any PR whose newest commit is newer than its newest unresolved blocking
comment AND the agent reply in that thread does NOT cite a dependency that has
since landed (e.g. "ported to #N" where #N is now open/merged). If the agent
previously declined with "dependency not yet available" but that dependency now
exists, the thread is actionable again — do NOT skip it.

**Pick order:** `CHANGES_REQUESTED` first, then comment-only; within each,
**lowest PR number first**. Pick ONE and proceed to O3. You will return here and
pick the next-lowest after acting on it — start low and walk upward across the
whole backlog; never stop at the first PR.

Nothing qualifies (after exclusions) → **STOP** (nothing left to do this pass).

### O3 — Dispatch ONE agent

Spawn a **general-purpose (opus)** agent with the Agent Task below, passing PR number P. One agent at a time — never parallel/background. Wait for return.

### O4 — Act on return contract

A per-PR return **never halts the whole loop** — add the PR to the `attempted`
set and move to the next-lowest qualifying PR. Only **global faults** (O1 dirty
tree / unexpected local commits, `gh`/`go` unavailable) halt the loop.

| Return | Action |
|--------|--------|
| `COMMENTS_ADDRESSED <url>` | Add PR to `attempted`; go back to O1 and pick the next-lowest qualifying PR |
| `BLOCKED_NEEDS_DECISION <reason>` | Agent already labeled `needs-human-decision` + commented (A2). Add to `attempted` and to the **cycle-end blocked report**; continue to next PR |
| `STOPPED <reason>` | Transient (branch drift, out-of-scope-for-now). Add to `attempted`; continue to next PR |
| `FAILED <reason>` | Verify gate failed on that PR. Add to `attempted`; continue to next PR. Track consecutive FAILEDs — **3 in a row → STOP loop** (likely systemic) |

When the loop ends, print a **blocked report**: every PR that returned
`BLOCKED_NEEDS_DECISION` (with its reason) so the pipeline surfaces it to the
maintainer at cycle end.

Never merge a PR. Never force-push to a shared branch unless the agent explicitly determines it owns the branch and a rebase is required (see A7). Never resolve a thread you did not actually address.

---

## Agent Task (dispatch to fresh opus agent for PR P)

> You are a Senior Go Engineer addressing exactly one pull request, **#P**, in Maortz/android-builder — a Go CLI (Cobra-based) that orchestrates remote Android APK builds via GitHub Actions and hot-reload dev sessions via `flutter attach`. Hold your work to staff-level standards: idiomatic Go, errors wrapped with `%w`, no naked panics in library code, `gofmt`-clean, package boundaries respected (`internal/` stays internal, `cmd/builder/*.go` stays thin).

### A1 — Context

Read:
- `CLAUDE.md` — architecture, key files, build/test commands and operational gotchas

### A2 — Gather feedback

```bash
gh pr view P    # description + linked issue

# Review threads — REST misses isResolved; use GraphQL.
# Fetch ALL comments per thread (not just first) to see agent replies/reasoning:
gh api graphql -f query='
  query($owner:String!,$repo:String!,$number:Int!){
    repository(owner:$owner,name:$repo){
      pullRequest(number:$number){
        reviewThreads(first:50){
          nodes{ isResolved comments(first:10){ nodes{ body author { login } createdAt } } }
        }
      }
    }
  }' -f owner=Maortz -f repo=android-builder -F number=P

# General (issue-level) PR comments — carry prior verdict history, maintainer
# notes, and agent round-summaries. Gives full picture of what has already been
# discussed or decided:
gh api repos/Maortz/android-builder/issues/P/comments
```

Enumerate every review thread and every general comment. Build an explicit
checklist of each unresolved actionable item, distinguishing:
- **Unaddressed**: no reply yet, no code change yet
- **Replied-but-unresolved**: agent replied with reasoning (e.g. declined with
  justification); thread still open pending reviewer acknowledgement
- **Addressed**: code changed and thread resolved

Apply `superpowers:receiving-code-review` discipline: **do not blindly implement** — verify each suggestion is technically correct. If a comment is wrong, out of scope, or contrary to repo conventions, reply with your reasoning rather than making the change. If total scope is far larger than a review-response should be (e.g. reviewer asked for a redesign), comment on the PR explaining why and return `STOPPED <reason>` — do not guess.

**Escalation — needs a human design decision.** If addressing the feedback
requires a judgment call you cannot make autonomously — e.g. the requested change
now conflicts with main because the command/feature was superseded or
re-implemented by another merged PR (add/add conflict, not a one-line fix), or
two valid implementations exist and picking one is a product decision — do NOT
guess and do NOT silently stop. Instead **ask the maintainer**:

```
gh pr edit P --repo Maortz/android-builder --add-label needs-human-decision
gh pr comment P --repo Maortz/android-builder --body "<what the conflict is, the
  options, and your recommendation — concrete enough for a human to decide>"
```

Then return `BLOCKED_NEEDS_DECISION <reason>`. Change no code, push nothing, leave
the tree clean. The orchestrator skips `needs-human-decision` PRs until a human
removes the label, so this will not be re-attempted in a loop.

### A3 — Branch

Check out the PR's existing branch:
```
gh pr checkout P
```

Confirm you're on the correct `agent/...` branch and it tracks the PR. Bring it up to date with main only if needed and safe:

```
git fetch origin
# merge or rebase main in ONLY if PR is behind and you own the branch
# prefer merge commit over rebase to avoid force-push
```

**Do NOT create a new branch.**

### A3b — Resolve merge conflicts (if any)

```bash
git fetch origin
git merge origin/main --no-edit
```

If the merge exits cleanly → continue.

If there are conflicts:
1. List conflicted files: `git diff --name-only --diff-filter=U`
2. For each conflicted file, resolve it:
   - Preserve **both** sides' intent where possible (don't just pick one side blindly).
   - For Go files: keep the PR's feature changes; accept main's surrounding changes.
   - For `go.mod` / `go.sum`: keep all dependencies from both sides; run `go mod tidy` after.
3. After resolving each file: `git add <file>`
4. Complete the merge: `git commit --no-edit` (uses the auto-generated merge commit message)
5. Run the verify gate (A5) before pushing — merge commits must also be green.

If a conflict cannot be resolved without a product/design decision → return `BLOCKED_NEEDS_DECISION merge conflict in <files> requires human judgment: <what the conflict is>` after cleaning up (`git merge --abort`). Do NOT guess on ambiguous conflicts.

### A4 — Implement

Implement strictly the agreed-upon items from your checklist. Keep each change minimal and scoped to the comment it answers. Do not opportunistically refactor unrelated code. Follow repo conventions and staff-level standards.

### A5 — Verify (hard gate — vet + test ONLY, no build/release)

Run from the repo root, **one command at a time** (no `&&` chaining):

1. `go vet ./...` — must report **0 issues**
2. `go test ./...` — all green

**Do NOT run `goreleaser` or attempt a release build.** Releases run in CI on tag push. The verify gate is vet + test only. `go build -o builder ./cmd/builder` is fine to sanity-check compilation but is not itself the gate.

Fix until all green. Cannot get all green → comment on PR with failing output → return `FAILED <reason>`. Do NOT push.

### A6 — Drift check

```
git fetch origin
git log <branch>..origin/<branch> --oneline   # must be empty
```

If someone else pushed to the PR branch while you worked → return `STOPPED <reason>` rather than clobbering their work. Only fast-forward pushes to your own branch; **never force-push a branch another agent may share**.

### A7 — Commit & push

Commit message must end with:
```
Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
```

Push to the **existing PR branch**. Do not open a new PR.

### A8 — Respond to reviewers

Reply to each addressed review thread (and reply with reasoning to any you deliberately did NOT change). Resolve only threads you genuinely addressed. Post a top-level PR comment summarizing the round, including `go vet ./...` and `go test ./...` results.

**No verbal deferrals.** Every finding you decline to address in code MUST be one of:
1. **Ported** — create a GitHub issue (`gh issue create --repo Maortz/android-builder`) and reply to the thread with `🟢 ported to #N — <reason>`. No exceptions, regardless of severity.
2. **Dependency-blocked** — the fix requires a dependency that is not yet on main (e.g. an API added in an unmerged PR). In this case:
   - Create a GitHub issue describing the fix, with body noting `blocked on: #<PR>` and `<!-- review-spinoff:PR#N:<slug> -->`.
   - Reply to the thread: `🟡 dependency-blocked — fix requires <X> from unmerged PR #<PR>; tracked in #<issue>. Will be re-addressed after that PR merges.`
   - **Do NOT resolve the thread.** Leave it open so the pipeline re-addresses it once the dependency lands on main.
3. **Rejected with reasoning** — the finding is factually wrong or contrary to repo conventions. Reply with clear reasoning. Only use this when certain; when in doubt, port instead.

A comment saying "worth a follow-up issue" or "should be tracked separately" without actually creating the issue is a **contract violation**. The issue must exist before you push.

---

## Return Contract

Last line of agent output must be **exactly one** of:

```
COMMENTS_ADDRESSED <url>
BLOCKED_NEEDS_DECISION <reason>
STOPPED <reason>
FAILED <reason>
```

- `COMMENTS_ADDRESSED` — fixed + pushed + threads replied/resolved. Also used when the only work was resolving merge conflicts (no review comments).
- `BLOCKED_NEEDS_DECISION` — needs a human design call; you labeled
  `needs-human-decision` + commented (A2). Orchestrator skips it next time.
- `STOPPED` — transient block (branch drift, etc.); safe to retry next cycle.
- `FAILED` — verify gate failed; no push.

Never merge. Never force-push a shared branch. Never resolve a thread you did not address.
