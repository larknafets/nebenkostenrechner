# Git workflow: repo specifics

The general rules (commit format, types, branches, PR or `main`, bundling, merge, push guard) are not kept in this repo. This file holds only what is specific here and what the hooks enforce.

## Commit messages

- **German**, with proper Umlaute (ä, ö, ü, ß), no ASCII substitutes (ae, oe, ue, ss). The type, the scope and technical names stay as they are (`feat`, `scripts/check`, `GITHUB_REF_NAME`).
- Verbs in the infinitive or as a noun phrase, not in the past tense: "Zeitraum auf volle Monate eingrenzen", not "Zeitraum eingegrenzt". First word of the summary capitalized.
- Format `<type>(<scope>): <Summary>`, the component name is the default scope (`abrechnung`, `nav`, `ha-addon`). The `commit-msg` hook rejects a wrong subject, one over 72 characters, a body line over 72, an em dash and common ASCII spellings of an Umlaut (`geaendert`).
- The release notes are not generated from the subjects: the user-facing text is `.changelog/unreleased.md` (`changelog.md`), the subject is for the developer.
- Examples: `feat(abrechnung): Zeitraum auf volle Monate eingrenzen für Mieterwechsel`, `fix(dashboard): Update-Punkt nur bei bekanntem Update zeigen`, `docs: Regel PR oder direkt auf main je nach Commit-Typ`.

## Product docs vs. steering docs

The `commit-msg` hook matches the type against the staged files:

- A `feat`, `fix`, `refactor` or `perf` commit may not touch `AGENTS.md`, `CLAUDE.md` or `docs/agents/`, and may not change only documentation. The scope `docs` needs the type `docs`.
- Product documentation of an epic (`README.md`, `GLOSSARY.md`, ADRs) is its own `docs(<scope>)` commit on the epic branch and merges with the PR, because on `main` it would describe behavior that is not there yet.
- The changelog line of a change is not a separate `docs` commit. It belongs to the commit of the change (`feat`, `fix`, or the `build` commit of a Wartung entry) and travels with its PR. `CHANGELOG.md` itself changes only at a release and when old entries are revised, both as a `docs` commit on `main`. A `build` change an operator must know about gets a changelog line too (`changelog.md`).

## Only through a PR

The pre-push hook refuses a push to `main` that changes any of these (`scripts/pr-only-paths`):

- `.github/workflows/`
- the Go version in `go.mod` and the `Dockerfile`: CI has the pinned linter and toolchain, a local run does not (Go 1.27.2 went straight to `main` and broke the pinned golangci-lint). Check the linter pin in `ci.yml` in the same change.

## Signing and merging

- The pre-push hook rejects unsigned commits and tags.
- Why `--rebase` is banned: PR #461 put five unsigned commits on `main`, GitHub re-creates rebased commits without a signature. A branch with merge commits (from `scripts/merge-ticket`) cannot be rebased anyway.

## Review bots

Sourcery posts a review minutes after the PR opens (`pending` in the check list means it is still writing). It is not a required check (branch protection requires only `test`), but its findings are read before auto-merge is switched on. A PR was once auto-merged with 8 unread findings, among them a test tied to the year 2026 and a missing "Vor dem Update" notice.
