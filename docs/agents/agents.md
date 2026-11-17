# Subagents: repo specifics

## Own models

- Subagents with their own model live in `.claude/agents/`. That folder is local to one machine and not versioned (`.claude/` is in `.gitignore`), so a fresh clone has none. Today: `ci-status` (Haiku): finds why a check or workflow run is red, read-only.

## Worktree scripts

- Check a finished ticket branch first with `scripts/review-ticket <branch>` on the integration branch: it prints the commits with their signature, the diffstat, changed golden files, every removed test line and added em dashes. A removed test line is a rename or a helper call, never an expectation.
- Merge a finished ticket branch into the integration branch with `scripts/merge-ticket <branch>` (merge, then `scripts/check`, stops at a conflict or a red check). It refuses every branch but `epic/*`, so check `git branch --show-current` first in its own step: another session may have switched the checkout.
- After the epic is merged, run `scripts/worktree-cleanup` from the main checkout (it refuses to run inside an agent worktree): it removes the agent worktrees under `.claude/worktrees` and the `ticket/*` and `worktree-agent-*` branches that are merged into the current branch, and lists what it kept. A backup branch is removed by hand.

## Implementer brief: additions

- Commit as `git-workflow.md` says (German, the commit-msg hook enforces the format).
- Changelog line in the same commit as a `feat` or `fix` (`changelog.md`), none for a `refactor`.
- The check command is `scripts/check`.

## Reviewer brief: additions

- Sources: `AGENTS.md`, `language-convention.md`, `git-workflow.md`, `changelog.md`, `GLOSSARY.md`, `docs/adr/`.
- Exclude generated golden files from the diff: `git diff <base>...HEAD -- . ':!internal/web/testdata'`.
- Proof for "tests and golden file unchanged": `git diff --stat <base>...HEAD -- internal/web/testdata` must be empty, and `git diff -U0 <base>...HEAD -- '*_test.go' | grep '^-[^-]'` shows only helper calls, never an expectation.
