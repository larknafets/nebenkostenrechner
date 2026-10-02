## Project

Web app for monthly utility cost billing for a two-family house with heat pump and PV system. A Home Assistant Add-on wrapping this app lives at https://github.com/larknafets/ha-addons, subfolder `nebenkostenrechner` (nightly: `nebenkostenrechner-nightly`), also accessible locally at `../ha-addons`.

## Commands and layout

- Run: `DB_PATH=./nk.db go run ./cmd/nebenkostenrechner` (`LISTEN_ADDR` default `:8080`, `WIDGET_LISTEN_ADDR` default `:8081`, `DB_PATH` default `/data/nebenkosten.db`, `LOGIN_PASSWORD` optional)
- Check before every commit: `scripts/check` (only the tests: `go test ./...`)
- `cmd/nebenkostenrechner`: main, wiring through environment variables
- `internal/calc`: pure cost calculation (Strom, Wasser, Heizung, Einspeisung, Fixkosten), no HTTP
- `internal/store`: SQLite (modernc), `schema.sql`, Ablesungen, Fixkosten-Eingaben, Stammdaten
- `internal/web`: handlers, Dashboard, Abrechnung, CSV; templates in `templates/` (embedded)
- Domain terms: `CONTEXT.md` (use its vocabulary), decisions: `docs/adr/`. User docs: `README.md`.

## Rules that always apply

- **Language**: German for GitHub issues, commit messages and user docs, English for code comments and `AGENTS.md`/`docs/agents/`. Details: `docs/agents/language-convention.md`.
- **Writing style**: no em dashes (—) in issues, comments, commit messages and committed files. Use commas, colons or " - ".
- **Plan mode**: make the plan extremely concise, sacrifice grammar for concision. End each plan with a list of unresolved questions, if any.
- **Git identity**: use the name and email already configured (`git config user.name` / `user.email`), never guess another one.
- **PR or main**: product changes (`feat`, `fix`, `refactor`, `perf`) go through a PR, never open one without asking. Developer changes (`docs`, `chore`, `ci`, `test`, `style`, `build`) go straight to `main`, no PR, no release. A release only when the user asks.
- **Push**: a global hook blocks `git push` for the agent. Commit locally, tell the user, the user pushes with `! git push`.
- **Changelog**: a `feat` or `fix`, or anything an operator must know when updating, adds one German line to `.changelog/unreleased.md` in the same commit (never as a separate `docs` commit, so a revert takes the line with it). Only the release itself touches `CHANGELOG.md`. Details: `docs/agents/releases.md`.
- **Checks**: run `scripts/check` before every commit. The pre-push hook (`git config core.hooksPath .githooks`) runs it too.

## Read when needed

| Topic | File |
|---|---|
| Branches, PR vs `main`, bundling, auto-merge | `docs/agents/git-workflow.md` |
| Commit message format | `docs/agents/git-commit-messages.md` |
| Changelog, tag, release, nightly, release checklist | `docs/agents/releases.md` |
| `scripts/check`, hooks, review before a PR, `actionlint` | `docs/agents/checks.md` |
| Subagents, Haiku, git inside a worktree | `docs/agents/agents.md` |
| Language per text type | `docs/agents/language-convention.md` |
| Issue tracker, Wayfinder commands | `docs/agents/issue-tracker.md` |
| Domain docs and glossary use | `docs/agents/domain.md` |
