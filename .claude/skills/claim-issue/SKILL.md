---
name: claim-issue
description: >
  Use when a parallel agent needs to atomically claim one agent-ready GitHub
  issue in Maortz/android-builder before starting implementation, to prevent
  two agents from picking the same issue. Triggers on "claim an issue",
  "pick and claim issue", or at the start of any parallel issue-implementer run.
---

# Claim Issue

## Overview

Adds an `agent-in-progress` label to one unclaimed `agent-ready` issue, acting as a soft lock before implementation starts. Because GitHub has no atomic test-and-set, this is a best-effort claim: add the label immediately, then re-read to verify no collision.

## Steps

### C1 — Ensure label exists (once per session)

```bash
gh label list --repo Maortz/android-builder | grep agent-in-progress \
  || gh label create agent-in-progress \
       --repo Maortz/android-builder \
       --description "Being implemented by an agent right now" \
       --color "E4E669"
```

### C2 — List available issues

```bash
gh issue list \
  --repo Maortz/android-builder \
  --state open \
  --label agent-ready \
  --json number,title,labels,url
```

**Filter out** any issue whose `labels` array contains `agent-in-progress` or `blocked`.

Also skip any issue with an open PR:
```bash
gh pr list --repo Maortz/android-builder --state open --search "issue-<N>"
```

**Priority order** (same as issue-implementer):
1. `phase:2-fix` > `phase:3-build` > `phase:4-verify`
2. `effort:S` > `effort:M` > `effort:L`
3. Tiebreak: lowest issue number

Nothing qualifies → output `NONE` and stop.

### C3 — Claim

```bash
gh issue edit <N> \
  --repo Maortz/android-builder \
  --add-label agent-in-progress
```

### C4 — Verify claim (collision detection)

Wait ~3 seconds, then re-read:
```bash
gh issue view <N> --repo Maortz/android-builder --json labels
```

The label should appear. If the issue somehow disappeared from `agent-ready` (another agent removed it) → go back to C2 and pick the next candidate.

### C5 — Return

Output exactly:
```
CLAIMED <N> <url>
```

or if nothing available:
```
NONE
```

## Release the claim

Remove the label when implementation ends (success or failure):

```bash
gh issue edit <N> \
  --repo Maortz/android-builder \
  --remove-label agent-in-progress
```

- **PR opened** → remove `agent-in-progress` (issue stays `agent-ready` until PR merges)
- **STOPPED / FAILED** → remove `agent-in-progress` (another agent can retry)

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Skipping C4 (verify) | Always re-read; collision is silent without it |
| Not releasing on failure | Always release in the STOPPED/FAILED path or the issue is stuck |
| Racing before label exists | C1 is idempotent — always run it |
