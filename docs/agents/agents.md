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
- Worktree agents had heredocs in Bash refused by the guard hook. They write files with the Write tool.
- After the epic is merged, remove what is left: `git worktree remove <path>`, then `git branch -D` for the `ticket/*` and `worktree-agent-*` branches and any backup branch.

## Guard hook

A global `PreToolUse` hook (`~/.claude/hooks/block-dangerous-git.sh`) matches every Bash command line against regex patterns, first an allowlist, then a blocklist.

- **Allowed**: `git push -u origin <branch>` for a feature branch (always with explicit remote and branch), `git branch -D <branch>`, and on `main` exactly `git push [-u] origin HEAD:main` (the whole command, nothing else in the same call).
- **Blocked**: every other push to `main`/`master`, bare `git push` and `git push origin HEAD` (the current branch may be `main`), force, delete, `--mirror`, `--all` and `+refspec` pushes, tag pushes (`vX.Y.Z`, `--tags`), `reset --hard`, `clean -f`, `checkout .`, `restore .`, deleting `main`. The user runs these with `! <command>`; a release tag is always pushed by the user.
- The match covers the whole command line. Text of a blocked command inside a commit message or heredoc blocks the call, and so does a compound command like `git push origin x && git switch main` (a trailing `main` matches). Write such text to a file with the Write tool, run the commands in separate calls. The push to `main` is allowed only as the whole command: a commit chained before it with `&&` is blocked, so commit first and push in the next call.
- A separate permission classifier ("auto mode") may still refuse `gh pr create`, `gh pr merge --auto` or a push. The allow rules for them live in `.claude/settings.local.json` (local, not versioned).
