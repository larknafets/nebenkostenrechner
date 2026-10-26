# Subagents and models

## Own models

- Subagents with their own model live in `.claude/agents/`. That folder is local to one machine and not versioned (`.claude/` is in `.gitignore`), so a fresh clone has none. Today: `ci-status` (Haiku): finds why a check or workflow run is red, read-only.
- Use Haiku only for tasks that read a lot and report briefly (CI logs, searching many files). Not for a single command (`git status`, one commit): starting a subagent costs more than the command. Fixed queries with a fixed result belong in a script, waiting belongs in a background loop (`until ...`), neither needs a model.
- Never give Haiku irreversible or outward actions (merge, delete a branch, push), refactors, reviews, `calc`, migrations or anything touching money or data.
- A rule in an agent's prompt ("read only") is not a block. If an agent must not write, deny the commands in the permission settings.
- The agent reports, the main agent checks the report before acting on it.

## Git inside a worktree

Subagents isolated in a git worktree must call `/usr/bin/git` instead of plain `git`. The RTK and caveman hooks rewrite `git ...` to `rtk git ...`, and Claude Code rejects that in an isolated agent because it cannot verify the worktree directory. `/usr/bin/git` is not rewritten and runs in the agent's own worktree. Tell the agent in its prompt.

- A worktree agent can start on another branch than the integration branch. Put it on the integration branch with `/usr/bin/git checkout -B <ticket-branch> <integration-branch>`. `reset --hard` is blocked by the guard hook.
- A heredoc is refused only when its body contains text matching a blocked git pattern (see Guard hook). Worktree agents still write files with the Write or Edit tool, not with shell redirects or `python3` scripts: the tools show the diff and keep the guard out of the way.
- After the epic is merged, remove what is left: `git worktree remove <path>`, then `git branch -D` for the `ticket/*` and `worktree-agent-*` branches and any backup branch.

## Implementer brief

A prompt for an implementer subagent (one ticket, own worktree) points here instead of repeating these rules. Name only the ticket, the integration branch and the context pointers (spec, neighbouring tickets, earlier commits).

- Start on the integration branch: `/usr/bin/git checkout -B ticket/<nr> <integration-branch>`. All git through `/usr/bin/git`.
- Read the ticket with `gh issue view <nr> --json title,body,labels,comments` (see `issue-tracker.md`).
- Write files with Write or Edit (see above).
- Build test-first with the `tdd` skill. A refactoring keeps the existing tests as its safety net: expectations never change, helpers and input fixtures may. Name every changed expectation in the report.
- Commit in German as `<type>(<scope>): <Summary>`, subject at most 72 characters, no em dash, Umlaute as ä, ö, ü, ß (the commit-msg hook rejects longer subjects, em dashes and common ASCII spellings like `geaendert`). Rules: `git-commit-messages.md`.
- Changelog line in the same commit as a `feat` or `fix` (`changelog.md`), none for a `refactor`.
- `scripts/check` green before every commit and again after the final merge of the integration branch into the ticket branch.
- Do not push, open a PR or close an issue. The main agent merges the ticket branch into the integration branch.
- Report: branch, commits, what stayed open, every deviation from the ticket.

## Reviewer brief

A reviewer subagent (read-only) gets the diff command and these sources, so the prompt does not list them: `AGENTS.md`, `language-convention.md`, `git-commit-messages.md`, `changelog.md`, `GLOSSARY.md`, `docs/adr/`.

- Exclude generated golden files from the diff (`git diff <base>...HEAD -- . ':!internal/web/testdata'`): they are large and carry no review signal. Run `scripts/check` for the rest instead of reading what tooling enforces.
- Standards axis: a violation cites its rule. Spec axis: a finding quotes the spec line.

## Guard hook

A global `PreToolUse` hook (`~/.claude/hooks/block-dangerous-git.sh`) matches every Bash command line against regex patterns, first an allowlist, then a blocklist.

- **Allowed**: `git push -u origin <branch>` for a feature branch (always with explicit remote and branch), `git branch -D <branch>`, on `main` exactly `git push [-u] origin HEAD:main`, and a release tag exactly `git push origin vX.Y.Z` (each the whole command, nothing else in the same call, no `&&`, no pipe).
- **Blocked**: every other push to `main`/`master`, bare `git push` and `git push origin HEAD` (the current branch may be `main`), force, delete, `--mirror`, `--all` and `+refspec` pushes, every other tag push (`--tags`, `refs/tags/...`, a tag with extra flags, deleting a tag), `reset --hard`, `clean -f`, `checkout .`, `restore .`, deleting `main`. The user runs these with `! <command>`. The agent pushes a release tag only when the user asked for the release and `main` is green (`releases.md`), and never works around the hook (no API call, no hook edit) if it blocks.
- The match covers the whole command line. Text of a blocked command inside a commit message or heredoc blocks the call, and so does a compound command like `git push origin x && git switch main` (a trailing `main` matches). Write such text to a file with the Write tool, run the commands in separate calls. The push to `main` is allowed only as the whole command: a commit chained before it with `&&` is blocked, so commit first and push in the next call.
- A separate permission classifier ("auto mode") may still refuse `gh pr create`, `gh pr merge --auto` or a push. The allow rules for them live in `.claude/settings.local.json` (local, not versioned).
