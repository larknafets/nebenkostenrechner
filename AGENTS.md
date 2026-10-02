## Project

Web app for monthly utility cost billing for a two-family house with heat pump and PV system. A Home Assistant Add-on wrapping this app lives at https://github.com/larknafets/ha-addons, subfolder `nebenkostenrechner` - also accessible locally at `../ha-addons`.

## Agent skills

- **Issue tracker**: GitHub Issues via `gh` CLI (larknafets/nebenkostenrechner). See `docs/agents/issue-tracker.md`.
- **Domain docs**: Single-context: root `CONTEXT.md` + `docs/adr/`. See `docs/agents/domain.md`.

## Models

- Subagents with their own model live in `.claude/agents/`. That folder is local to one machine and not versioned (`.claude/` is in `.gitignore`), so a fresh clone has none. Today: `ci-status` (Haiku): finds why a check or workflow run is red, read-only.
- Use Haiku only for tasks that read a lot and report briefly (CI logs, searching many files). Not for a single command (`git status`, one commit): starting a subagent costs more than the command. Fixed queries with a fixed result belong in a script, waiting belongs in a background loop (`until ...`), neither needs a model.
- Never give Haiku irreversible or outward actions (merge, delete a branch, push), refactors, reviews, `calc`, migrations or anything touching money or data.
- A rule in an agent's prompt ("read only") is not a block. If an agent must not write, deny the commands in the permission settings.
- The agent reports, the main agent checks the report before acting on it.

## Checks before a push

- `scripts/check` runs the same steps as the CI job `test` (gofmt, vet, golangci-lint v2.13.2, test, build) plus `actionlint` for the workflows, `scripts/check --vuln` adds govulncheck. Run it before every commit that goes to `main`. It needs `golangci-lint` and `actionlint` on the PATH (`brew install golangci-lint actionlint govulncheck`).
- `git config core.hooksPath .githooks` (once per clone) turns on a pre-push hook that runs it, so a red state never reaches GitHub.
- Before opening a PR, review the diff locally with `/review` instead of waiting for a review bot on the PR. Sourcery on GitHub is optional and not a required check.

## PR or straight to main

- **Product changes** (`feat`, `fix`, `refactor`, `perf`) go through a PR. Never open one without asking first. A release is made only when the user asks for it.
- **Developer changes** (`docs`, `chore`, `ci`, `test`, `style`, `build`) need no PR and no release: commit on `main` after `scripts/check` is green, the user pushes (`! git push`, the guard blocks it for the agent). Branch protection lets the admin push to `main`, and `.goreleaser.yaml` keeps these types out of the release notes.
- A change under `.github/workflows/` is the one risky case: `actionlint` in `scripts/check` catches syntax, not behavior. If a workflow change cannot be judged from the diff, use a PR so CI runs before it lands.
- Check that `main` is green after a direct push before tagging a release.

## Plan mode

- Make the plan extremely concise. Sacrifice grammar for the sake of concision.
- At the end of each plan, give me a list of unresolved questions to answer, if any.

## Writing style

- No em dashes (—) in GitHub issue titles, bodies, comments, commit messages, and committed files (e.g. prototype HTML). Use commas, colons, or regular hyphens (" - ").

## Git rules

- **Author identity**: Always use the name and email already configured for the GitHub account in use (`git config user.name` / `user.email`, or the target repo's existing committer identity). Never assume, guess, or substitute a different identity (e.g. a system/session email) for commit author or committer.
- **Commit messages**: `docs/agents/git-commit-messages.md`
- **Branch naming**: `docs/agents/git-branches.md`
- **Releases and versioning**: `docs/agents/git-releases.md`
- **Agents in a worktree**: Subagents isolated in a git worktree must call `/usr/bin/git` instead of plain `git`. The RTK and caveman hooks rewrite `git ...` to `rtk git ...`, and Claude Code rejects that in an isolated agent because it cannot verify the worktree directory. `/usr/bin/git` is not rewritten and runs in the agent's own worktree. Tell the agent in its prompt.

## Language convention

`docs/agents/language-convention.md`
