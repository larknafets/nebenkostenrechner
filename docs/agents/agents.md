# Subagents and models

## Own models

- Subagents with their own model live in `.claude/agents/`. That folder is local to one machine and not versioned (`.claude/` is in `.gitignore`), so a fresh clone has none. Today: `ci-status` (Haiku): finds why a check or workflow run is red, read-only.
- Use Haiku only for tasks that read a lot and report briefly (CI logs, searching many files). Not for a single command (`git status`, one commit): starting a subagent costs more than the command. Fixed queries with a fixed result belong in a script, waiting belongs in a background loop (`until ...`), neither needs a model.
- Never give Haiku irreversible or outward actions (merge, delete a branch, push), refactors, reviews, `calc`, migrations or anything touching money or data.
- A rule in an agent's prompt ("read only") is not a block. If an agent must not write, deny the commands in the permission settings.
- The agent reports, the main agent checks the report before acting on it.

## Worktree scripts

Git use inside a worktree (`/usr/bin/git`, branch setup) follows the general git rules.

- Check a finished ticket branch first with `scripts/review-ticket <branch>` on the integration branch: it prints the commits with their signature, the diffstat, changed golden files, every removed test line and added em dashes. A removed test line is a rename or a helper call, never an expectation.
- Merge a finished ticket branch into the integration branch with `scripts/merge-ticket <branch>` (merge, then `scripts/check`, stops at a conflict or a red check). It refuses every branch but `epic/*`, so check `git branch --show-current` first in its own step: another session may have switched the checkout.
- After the epic is merged, run `scripts/worktree-cleanup` from the main checkout (it refuses to run inside an agent worktree): it removes the agent worktrees under `.claude/worktrees` and the `ticket/*` and `worktree-agent-*` branches that are merged into the current branch, and lists what it kept. A backup branch is removed by hand.

## Implementer brief

A prompt for an implementer subagent (one ticket, own worktree) points here instead of repeating these rules. Name only the ticket, the integration branch and the context pointers (spec, neighbouring tickets, earlier commits).

- Start on the integration branch (`ticket/<nr>`), all git through `/usr/bin/git`.
- Read the ticket with `gh issue view <nr> --json title,body,labels,comments`.
- Write files with Write or Edit.
- Before naming a new type or module, search the spec, the map notes and the open tickets for that name: a ticket that comes later may reserve it. Say in the report which names you chose.
- Build test-first with the `tdd` skill. A refactoring keeps the existing tests as its safety net: expectations never change, helpers and input fixtures may. Name every changed expectation in the report.
- Commit as `git-workflow.md` says (German, the commit-msg hook enforces the format).
- Changelog line in the same commit as a `feat` or `fix` (`changelog.md`), none for a `refactor`.
- `scripts/check` green before every commit and again after the final merge of the integration branch into the ticket branch.
- Do not push, open a PR or close an issue. The main agent merges the ticket branch into the integration branch.
- Report: branch, commits, what stayed open, every deviation from the ticket, and the test that failed before the change with its failure message.

## Reviewer brief

A reviewer subagent (read-only) gets the diff command and these sources, so the prompt does not list them: `AGENTS.md`, `language-convention.md`, `git-workflow.md`, `changelog.md`, `GLOSSARY.md`, `docs/adr/`.

- Exclude generated golden files from the diff (`git diff <base>...HEAD -- . ':!internal/web/testdata'`): they are large and carry no review signal. Run `scripts/check` for the rest instead of reading what tooling enforces.
- A spec that says "tests and golden file unchanged" needs proof, not the implementer's word: name `git diff --stat <base>...HEAD -- internal/web/testdata` (must be empty) and `git diff -U0 <base>...HEAD -- '*_test.go' | grep '^-[^-]'` (every removed test line must be a helper call, never an expectation).
- Done when every changed file of the diff (generated golden files and images excepted) is read. Name each file you did not read under "Not read" in the report, never skip one silently.
- Standards axis: a violation cites its rule. Spec axis: a finding quotes the spec line.
