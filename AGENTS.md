## Project

Web app for monthly utility cost billing for a two-family house with heat pump and PV system. A Home Assistant Add-on wrapping this app lives at https://github.com/larknafets/ha-addons, subfolder `nebenkostenrechner` (nightly: `nebenkostenrechner-nightly`), also accessible locally at `../ha-addons`.

## Commands and layout

- Run: `DB_PATH=./nk.db go run ./cmd/nebenkostenrechner` (`LISTEN_ADDR` default `:8080`, `WIDGET_LISTEN_ADDR` default `:8081`, `DB_PATH` default `/data/nebenkosten.db`, `LOGIN_PASSWORD` optional)
- `cmd/nebenkostenrechner`: main, wiring through environment variables
- `internal/calc`: pure cost calculation (Strom, Wasser, Heizung, Einspeisung, Fixkosten), no HTTP
- `internal/store`: SQLite (modernc), `schema.sql`, Ablesungen, Fixkosten-Eingaben, Stammdaten
- `internal/web`: handlers, Dashboard, Abrechnung, CSV; templates in `templates/` (embedded)
- Domain terms: `GLOSSARY.md` (use its vocabulary), decisions: `docs/adr/`. User docs: `README.md`.

## Rules that always apply

- **Language**: German for GitHub issues, commit messages and user docs, English for code comments and `AGENTS.md`/`docs/agents/`. Details: `docs/agents/language-convention.md`.
- **Writing style**: no em dashes (—) in issues, comments, commit messages and committed files. Use commas, colons or " - ".
- **Plan mode**: make the plan extremely concise, sacrifice grammar for concision. End each plan with a list of unresolved questions, if any.
- **Git identity**: use the name and email already configured (`git config user.name` / `user.email`), never guess another one.
- **PR or main**: product changes (`feat`, `fix`, `refactor`, `perf`) go through a PR, ask before opening one. Developer changes (`docs`, `chore`, `ci`, `test`, `style`, `build`) go straight to `main`, no PR, no release. A release only when the user asks.
- **Push**: push a feature branch with `git push -u origin <branch>`, `main` only with exactly `git push origin HEAD:main`, a release tag only with exactly `git push origin vX.Y.Z`. A global hook blocks everything else (other tag forms, force, bare `git push`, other forms on `main`), the user runs those with `! <command>`. Details: `docs/agents/agents.md`.
- **Changelog**: a `feat` or `fix`, or anything an operator must know when updating, adds one German line to `.changelog/unreleased.md` in the same commit (never as a separate `docs` commit, so a revert takes the line with it). Only the release itself touches `CHANGELOG.md`. Details: `docs/agents/changelog.md`.
- **Checks**: run `scripts/check` before every commit (`go test ./...` runs only the tests). The pre-push hook (`git config core.hooksPath .githooks`) runs it too.

## Read when needed

| When | File |
|---|---|
| Creating a branch, choosing PR or `main`, bundling, auto-merge | `docs/agents/git-workflow.md` |
| Writing a commit message, or a command line containing a blocked git command | `docs/agents/git-commit-messages.md`, `docs/agents/agents.md` |
| Adding a changelog line, writing release notes | `docs/agents/changelog.md` |
| Releasing: version, tag, nightly, checklist | `docs/agents/releases.md` |
| Before a commit or PR: `scripts/check`, hooks, review, `actionlint` | `docs/agents/checks.md` |
| Starting a subagent, Haiku, git inside a worktree | `docs/agents/agents.md` |
| Writing any text: which language | `docs/agents/language-convention.md` |
| Working with issues or Wayfinder | `docs/agents/issue-tracker.md` |
| Naming a domain concept, touching an ADR | `docs/agents/domain.md` |
