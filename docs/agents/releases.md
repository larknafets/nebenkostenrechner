# Releases and versioning

## Version number

`MAJOR.MINOR.PATCH`, the number is a promise to the user who updates.

- **MAJOR**: breaking change, e.g. a database migration that cannot be undone or a removed feature.
- **MINOR**: new feature, backward-compatible (a `feat` in the release).
- **PATCH**: bug fix only (`fix`, `build`), nothing new for the user.
- When unsure, ask the user. The user decides the number (once a `feat` shipped as patch `v0.14.1` on request).

## Release checklist

A release is made only when the user asks. The tag is the source of truth, nothing hand-edits a version.

1. `main` is green: `gh run list --branch main --limit 3`. Check after any direct push too.
2. Look at the commits since the last tag (`git log --oneline <last tag>..main`), propose the version.
3. Prepare the changelog with `scripts/release-prepare vX.Y.Z --dry-run`, check the shown section and the version hint with the user, then run it without `--dry-run`. It checks that `main` is clean, current and green, runs `scripts/check`, moves `.changelog/unreleased.md` into `CHANGELOG.md` as `## vX.Y.Z - YYYY-MM-DD` (empty sections left out, the collective lines for refactorings and Dependabot added), empties `unreleased.md`, commits as `docs` and sets the tag locally. It never pushes. The user pushes the commit, `main` must be green again before the tag goes out.
4. The script has set the tag `vX.Y.Z` (by hand: `git tag -a vX.Y.Z -m "vX.Y.Z"`).
5. The user pushes the tag: `! git push origin vX.Y.Z` (the global guard blocks it for the agent). A plain push sends no tags.
6. Wait for `release.yml` and `ha-addon.yml` (background `until` loop, no sleep chains). `release.yml` fails at the start if `CHANGELOG.md` has no `## vX.Y.Z` entry.
7. Verify and report:
   - release notes on GitHub (the section of this version, the footer `Alle Änderungen im Vergleich: vA...vB`)
   - `larknafets/ha-addons` `nebenkostenrechner/config.yaml` shows `version: "X.Y.Z"` (mirrored by the bot)
   - image `ghcr.io/larknafets/nebenkostenrechner:X.Y.Z` exists, `Validate Add-ons` in `ha-addons` is green
8. Remind the user: back up the database before updating. Never claim a check ran that did not.

A wrong tag that is not pushed yet is deleted locally (`git tag -d`). A pushed tag is never moved without asking.

## Channels: release and nightly

- **Release** is a tag `v*`: `release.yml` builds the image and creates the GitHub release from `CHANGELOG.md`, `ha-addon.yml` mirrors the version into `larknafets/ha-addons`, folder `nebenkostenrechner`.
- **Nightly** is `main` from the last night: `nightly.yml` (02:17 UTC, skipped without a commit in the last 24 hours, or started by hand for any branch) pushes the image tags `nightly` and `nightly.YYYYMMDD-<sha7>` and writes the dated version into `larknafets/ha-addons`, folder `nebenkostenrechner-nightly`. Only the newest 7 dated images are kept.
- No separate nightly or release branch: `main` is the nightly, a tag is the release.
- The nightly add-on has its own slug, its own data and the host port 8082 for the widget routes (release: 8081).

## Changelog

- `.changelog/unreleased.md` collects the entries of the next release, `CHANGELOG.md` is the running history (newest release first). The rules for the wording are the comment at the top of `unreleased.md`.
- Add the line in the **same commit** as the change, for anything a user or the operator of the Home Assistant app can see or feel. German, in the words of the user interface, from the user's side, with `(#nr)` if an issue drove it.
- Sections, always in this order, none removed: `## Neue Funktionen`, `## Verbesserungen`, `## Fehlerkorrekturen`, `## Wartung`.
- **Wartung** is what an operator must know when updating: Go version, base image, environment variables, ports, data paths, migrations, security updates. Dependabot updates are **one** line ("Aktualisierung der Abhängigkeiten"), added at the release if Dependabot commits landed since the last tag (`git log <last tag>..main --author=dependabot`), not per update. A `refactor` since the last tag gives **one** more line in Wartung, added at the release too: "Interne Aufräumarbeiten am Code ohne sichtbare Änderung" (`git log <last tag>..main --grep "^refactor"`), never one line per refactoring.
- No line for `docs`, `test`, `style`, `ci`, scripts, hooks, agent rules or a pipeline repair nobody notices. Refactorings only as the one collective line above.
- At the release, `CHANGELOG.md` gets the new section with the entries (sections as `###`, empty sections left out), `unreleased.md` is emptied but keeps the comment and all four headings.
- The history can be revised later at any time: `CHANGELOG.md` is a plain file, the text of an old GitHub release is changed with `gh release edit <tag> --notes-file <file>`. The `ha-addons` changelog is rebuilt from all releases at every release, so it follows.

## Release notes

`release.yml` creates the GitHub release with `gh release create`. The text is the section of this version from `CHANGELOG.md` (`###` becomes `##`) plus the German footer `Alle Änderungen im Vergleich: [vA...vB](compare link)`. The Home Assistant changelog in `ha-addons` is built from those texts by `ha-addon.yml`. There are no binaries and no checksums, the image from the `Dockerfile` is the artifact.
