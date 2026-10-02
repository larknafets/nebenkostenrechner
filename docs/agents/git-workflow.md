# Git workflow

## Product changes: PR. Developer changes: straight to `main`

- **Product changes** (`feat`, `fix`, `refactor`, `perf`) go through a PR so CI runs before the merge. Never open a PR without asking first.
- **Developer changes** (`docs`, `chore`, `ci`, `test`, `style`, `build`) need no PR and no release. Commit on `main` after `scripts/check` is green, the user pushes (`! git push`, the global guard blocks it for the agent). Branch protection lets the admin push to `main` (`enforce_admins` is off, GitHub prints "Bypassed rule violations"). They get no line in the changelog either, except a `build` change an operator must know about (`releases.md`).
- `style` means formatting only (whitespace, indentation, `gofmt`), behavior stays identical. A CSS or layout change users can see is a `fix` or `feat`, a rename without behavior change is a `refactor`. If unsure, do not use `style`.
- Exception to "`docs` goes straight to `main`": the changelog line of a change is not a separate `docs` commit. It belongs to the commit of the change (`feat`, `fix`, or the `build` commit of a Wartung entry) and travels with its PR. `CHANGELOG.md` itself changes only at a release and when old entries are revised, both as a `docs` commit on `main`.
- A change under `.github/workflows/` is the one risky developer change: `actionlint` catches syntax, not behavior. If the diff does not show how it behaves, use a PR so CI runs before it lands.
- After a direct push check that `main` is green (`gh run list --branch main`) before tagging a release.

## Branches

- Branch from `main`, keep branches short-lived (merge within 1-3 days), delete after merge (except `prototype/*`).
- Prefer feature flags over long-lived branches for incomplete features.
- Name: `<type>/<description>`, the type is the commit type of the change.

| Type | Purpose | Example |
|---|---|---|
| `feat/` | New feature | `feat/abrechnung-zeitraum` |
| `fix/` | Bug fix | `fix/update-punkt` |
| `refactor/` | Refactoring | `refactor/saldo-bundeln` |
| `chore/` | Tooling, dependencies | `chore/lokale-checks` |
| `docs/` | Documentation | `docs/commit-sprache` |
| `epic/` | Bundle of one Wayfinder map or spec | `epic/nightly-kanal` |
| `prototype/` | Prototype for a decision, kept | `prototype/login-dashboard` |
| `research/` | Research result of a Wayfinder ticket, kept | `research/ha-kanal` |

`dependabot/*` is created by Dependabot. `main` needs no prefix.

## Bundling: fewer PRs

- **One Wayfinder map or spec = one `epic/<name>` branch and one PR to `main`.** Tickets and subagents commit onto it. A subagent in its own worktree branch is merged into the epic branch by the main agent and opens no PR.
- **Small unrelated product tweaks of one session** (labels, order of fields, layout) go onto one branch with one PR at the end of the session.
- One logical change per commit (`git-commit-messages.md`). The release notes come from `CHANGELOG.md`, not from the commit messages: a PR with a user-visible change carries its line in `.changelog/unreleased.md`.
- Merge a multi-commit PR with `gh pr merge --auto --rebase` so every commit reaches `main` with its own message. Squash-merge only a single-commit PR.
- Switch on auto-merge when the PR is opened (`--auto`), then it merges as soon as the `test` check is green. The branch must be up to date with `main` (strict check): if it falls behind, `gh pr update-branch <nr> --rebase`.
- Do not push to a branch whose PR is already merged, the commit would miss `main`. Check `gh pr view` first.
