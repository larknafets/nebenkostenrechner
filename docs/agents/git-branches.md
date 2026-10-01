# Branch naming

- Branch from `main`
- Keep branches short-lived (merge within 1-3 days) — long-lived branches are hidden costs
- Delete branches after merge, except `prototype/*`
- Prefer feature flags over long-lived branches for incomplete features

Branch names follow this structure:

```
<type>/<description>
```

| Type | Purpose | Example |
|---|---|---|
| `feature/` or `feat/` | New features | `feature/add-login-page` |
| `bugfix/` or `fix/` | Bug fixes | `fix/header-bug` |
| `hotfix/` | Urgent fixes | `hotfix/security-patch` |
| `release/` | Release preparation | `release/v1.2.0` |
| `chore/` | Non-code tasks | `chore/update-dependencies` |
| `epic/` | Bundle of one Wayfinder map or spec | `epic/nightly-kanal` |
| `prototype/` | Design prototypes for decisions | `prototype/login-dashboard` |

Trunk branches (`main`, `master`, `develop`) do not require a prefix.

## Bundling work: one PR per map

Too many small PRs cost review and wait time. Bundle:

- **One Wayfinder map or spec = one `epic/<name>` branch and one PR to `main`.** Tickets and subagents commit onto it. If a subagent works in its own worktree branch, the main agent merges that branch into the epic branch, the subagent opens no PR.
- **Small unrelated tweaks of one session** (labels, order of fields, layout) are collected on one branch with one PR at the end of the session.
- **Keep one logical change per commit** (see `git-commit-messages.md`). The release notes are generated from the commit messages.
- **Merge a multi-commit PR with `gh pr merge --auto --rebase`** so every commit reaches `main` and the changelog. Squash-merge only a single-commit PR, otherwise the changelog gets one line (the PR title) for all of it.
- **Switch on auto-merge when the PR is opened** (`--auto`), then it merges as soon as the `test` check is green.
- Delete the branch after the merge (`prototype/*` excepted).
