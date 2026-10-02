# Commit messages

Conventional Commits, in German, terse and exact. Why over what. The release notes are not generated from the subjects (the user-facing text is `.changelog/unreleased.md`, see `releases.md`), the subject is for the developer.

## Language

- German, with proper Umlaute (ä, ö, ü, ß). No ASCII substitutes (ae, oe, ue, ss).
- The type, the scope and technical names stay as they are (`feat`, `fix`, `scripts/check`, `GITHUB_REF_NAME`).
- Verbs in the infinitive or as a noun phrase, not in the past tense: "Zeitraum auf volle Monate eingrenzen", not "Zeitraum eingegrenzt".

## Splitting commits

- One commit = one logical change. Split when a diff mixes unrelated concerns (e.g. a feature and a tooling change), even if done in the same session.
- Each commit passes on its own where feasible.
- Do not split one feature's code and its own tests into two commits.

## Subject line

- `<type>(<scope>): <summary>`, first word of the summary capitalized, no trailing period, aim for at most 50 characters, hard cap 72.
- Types, always lower case:
  - `feat`: new feature
  - `fix`: bug fix
  - `refactor`: restructuring without a change of behavior
  - `perf`: performance improvement
  - `docs`: documentation (README, `AGENTS.md`, `CONTEXT.md`, ADRs)
  - `test`: new or corrected tests
  - `style`: formatting only (whitespace, `gofmt`), behavior identical
  - `chore`: tooling and housekeeping that touches neither the app nor its tests (e.g. `scripts/`, hooks)
  - `ci`: files under `.github/workflows/` (when and how the pipeline runs)
  - `build`: the packaging tool's own config (`Dockerfile`) or external dependencies
  - `revert`: reverts a commit
- A bug in a `ci` or `build` file keeps that type, it is not a `fix`.
- Which types need a PR and which go straight to `main`: `git-workflow.md`.
- The scope is lower case and may be empty. The component name is the default scope (`abrechnung`, `nav`, `ha-addon`). Only for a cross-cutting change with an issue number use `#<nr>` instead, never both.
- The issue reference in the scope does not replace the reference at the end of the body (`Closes #42`, `Refs #17`).

## Body (only if needed)

- Skip it when the subject is clear.
- Write it for: the non-obvious why, breaking changes, migration notes, linked issues. Always for breaking changes, security fixes, data migrations and reverts.
- Wrap at 72 characters, bullets with `-`, references to issues at the end.

## Never

- "Dieser Commit ...", "wir", "jetzt", "aktuell": the diff says what.
- "Auf Wunsch von ...".
- "Generated with Claude Code" or any AI attribution, no emoji.
- The file name again when the scope already says it.
- The literal text "git push" (and the other blocked commands, `agents.md`) in the command line that creates the commit.

## Examples

- ✅ `feat(abrechnung): Zeitraum auf volle Monate eingrenzen für Mieterwechsel`
- ✅ `fix(dashboard): Update-Punkt nur bei bekanntem Update zeigen`
- ✅ `docs: Regel PR oder direkt auf main je nach Commit-Typ`
- ❌ `feat: Eine neue Funktion hinzugefügt, mit der man den Zeitraum ändern kann`

With a body:

```
feat(abrechnung): Zeitraum auf volle Monate eingrenzen für Mieterwechsel

Häkchen blendet Von-/Bis-Monat ein, Standard bleibt das Kalenderjahr.
Die Frist nach § 556 Abs. 3 BGB hängt am Zeitraumende.

Closes #189
```

Breaking change:

```
feat(web)!: Pfad /abrechnung nach /jahresabrechnung umbenennen

BREAKING CHANGE: Lesezeichen auf /abrechnung funktionieren nicht mehr.
```
