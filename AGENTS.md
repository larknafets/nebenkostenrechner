## Project

Web app for monthly utility cost billing for a two-family house with heat pump and PV system. A Home Assistant Add-on wrapping this app lives at https://github.com/larknafets/ha-addons, subfolder `nebenkostenrechner` (nightly: `nebenkostenrechner-nightly`), also accessible locally at `../ha-addons`.

## Commands and layout

- Run: `DB_PATH=./nk.db go run ./cmd/nebenkostenrechner` (`VERMIETER_PASSWORD` and `MIETER_PASSWORD` optional, other variables: `cmd/nebenkostenrechner/main.go`)
- Look at it in the browser: `scripts/dev-server` (log in as Vermieter with password `dev`, a private window shows the not-logged-in view)
- `cmd/nebenkostenrechner`: main, wiring through environment variables
- `internal/calc`: pure cost calculation (Strom, Wasser, Heizung, Einspeisung, Fixkosten), no HTTP
- `internal/store`: SQLite (modernc), `schema.sql`, Ablesungen, Fixkosteneingaben, Stammdaten
- `internal/web`: handlers, Dashboard, Abrechnung, CSV; templates in `templates/` (embedded)
- Domain terms: `GLOSSARY.md` (use its vocabulary), decisions: `docs/adr/`. User docs: `README.md`.

## Rules that always apply

- **Language**: German for GitHub issues, commit messages and user docs, English for code comments and `AGENTS.md`/`docs/agents/`.
- **Releases**: a release only when the user asks.
- **Changelog**: a `feat` or `fix`, or anything an operator must know when updating, adds one German line to `.changelog/unreleased.md` in the same commit (never as a separate `docs` commit, so a revert takes the line with it). Only the release itself touches `CHANGELOG.md`.

## Read when needed

| When | File |
|---|---|
| Writing a commit message (German, hook rules), choosing PR or `main` for the Go version, `Dockerfile`, workflows, docs vs. product commits, signing, review bots | `docs/agents/git-workflow.md` |
| Adding a changelog line, writing release notes | `docs/agents/changelog.md` |
| Releasing: version, tag, nightly, checklist | `docs/agents/releases.md` |
| Before a commit or PR: `scripts/check`, hooks, `scripts/dev-server`, `scripts/smoke` | `docs/agents/checks.md` |
| Starting an implementer or reviewer subagent, Haiku, merging ticket branches | `docs/agents/agents.md` |
| Writing any text: which language | `docs/agents/language-convention.md` |
| Pipeline order (`/wayfinder`, `/to-spec`, `/to-tickets`, `/implement-spec`, `/ponytail-review`), writing a spec or tickets | `docs/agents/pipeline.md` |
