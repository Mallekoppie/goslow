# Issue tracker: GitHub Issues

Issues and specs for this repo live in GitHub Issues, managed with the authenticated `gh` CLI.

## Prerequisites

- `gh` is installed by the devcontainer (see `.devcontainer/README.md`).
- Authenticate once per container: `gh auth login`.

## Conventions

- One issue per ticket. The issue body is the spec: what to build end to end, plus its acceptance criteria.
- Triage state is a label from `triage-labels.md`. The agent-grabbable state is `ready-for-agent`.
- Blocking relationships live in the issue body, because GitHub's native issue dependencies are not available through `gh` on every plan:

  ```text
  Blocked by: #12, #14
  ```

  An issue is unblocked when every issue it lists is closed.
- Implementation notes and conversation append as issue comments; do not rewrite the body to record progress.
- Do not reopen a closed issue to record follow-up work. File a new issue and reference the old one.

## When a skill says "publish to the issue tracker"

Create one issue per ticket in dependency order, blockers first:

```bash
gh issue create --title "<title>" --body-file <body.md> --label ready-for-agent
```

## When a skill says "fetch the relevant ticket"

```bash
gh issue view <number> --comments \
  --json number,title,body,labels,state,comments
```

## Resolving work

Comment the implementation summary and verification, then close the issue:

```bash
gh issue comment <number> --body "<summary and verification>"
gh issue close <number>
```

## Wayfinding operations

Used by `/wayfinder`. The map and each child ticket are GitHub issues.

- **Map**: one open issue labelled `wayfinder-map`. Its body holds the notes, decisions so far, and fog.
- **Child ticket**: one issue per question. A `Type:` line in the body records `research`/`prototype`/`grilling`/`task`; `Blocked by: #NN` lines record dependencies.
- **Frontier**: `gh issue list --state open --label ready-for-agent`; skip issues whose `Blocked by` issues are still open; first by number wins.
- **Claim**: add the `claimed` label (create it with `gh label create claimed` if absent) and comment before starting.
- **Resolve**: comment the answer, close the issue, and append a context pointer to the map issue.
