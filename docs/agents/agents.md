# Subagents and models

## Own models

- Subagents with their own model live in `.claude/agents/`. That folder is local to one machine and not versioned (`.claude/` is in `.gitignore`), so a fresh clone has none. Today: `ci-status` (Haiku): finds why a check or workflow run is red, read-only.
- Use Haiku only for tasks that read a lot and report briefly (CI logs, searching many files). Not for a single command (`git status`, one commit): starting a subagent costs more than the command. Fixed queries with a fixed result belong in a script, waiting belongs in a background loop (`until ...`), neither needs a model.
- Never give Haiku irreversible or outward actions (merge, delete a branch, push), refactors, reviews, `calc`, migrations or anything touching money or data.
- A rule in an agent's prompt ("read only") is not a block. If an agent must not write, deny the commands in the permission settings.
- The agent reports, the main agent checks the report before acting on it.

## Git inside a worktree

Subagents isolated in a git worktree must call `/usr/bin/git` instead of plain `git`. The RTK and caveman hooks rewrite `git ...` to `rtk git ...`, and Claude Code rejects that in an isolated agent because it cannot verify the worktree directory. `/usr/bin/git` is not rewritten and runs in the agent's own worktree. Tell the agent in its prompt.

## Guard hook

A global `PreToolUse` hook (`~/.claude/hooks/block-dangerous-git.sh`) blocks `git push`, `reset --hard`, `clean -f`, `branch -D`, `checkout .` and `restore .` for every agent. It matches the whole command line, so even the text "git push" inside a heredoc or a commit message is blocked: write such files with the Write tool and phrase the message differently. The user runs these commands with `! <command>`.
