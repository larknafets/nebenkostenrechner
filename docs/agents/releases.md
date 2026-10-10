# Releases and versioning

## Version number

`MAJOR.MINOR.PATCH`, the number is a promise to the user who updates.

- **MAJOR**: breaking change, e.g. a database migration that cannot be undone or a removed feature.
- **MINOR**: new feature, backward-compatible (a `feat` in the release).
- **PATCH**: bug fix only (`fix`, `build`), nothing new for the user.
- When unsure, ask the user. The user decides the number (once a `feat` shipped as patch `v0.14.1` on request).

## Release checklist

A release is made only when the user asks. The tag is the source of truth, nothing hand-edits a version.

1. After any direct push to `main`, check it is green (`gh run list --branch main --limit 3`). `scripts/release-prepare` checks it again.
2. Look at the commits since the last tag (`git log --oneline <last tag>..main`), propose the version.
3. Prepare the changelog with `scripts/release-prepare vX.Y.Z --dry-run`, check the shown section and the version hint with the user, then run it without `--dry-run`. It checks that `main` is clean, current and green, runs `scripts/check`, moves `.changelog/unreleased.md` into `CHANGELOG.md` as `## vX.Y.Z - YYYY-MM-DD` (empty sections left out, the collective lines for refactorings and Dependabot added), empties `unreleased.md`, commits as `docs` and sets the tag locally (by hand: `git tag -a vX.Y.Z -m "vX.Y.Z"`). It never pushes. Push the commit with `git push origin HEAD:main`, `main` must be green again before the tag goes out.
4. Once `main` is green again, push the tag with exactly `git push origin vX.Y.Z`, as its own call (the guard allows only that form). If the guard blocks it anyway, the user pushes it with `! git push origin vX.Y.Z`. A plain push sends no tags.
5. Wait for `release.yml` and `ha-addon.yml` (background `until` loop, no sleep chains). `release.yml` fails at the start if `CHANGELOG.md` has no `## vX.Y.Z` entry.
6. Verify and report:
   - release notes on GitHub (the section of this version, the footer `Alle Änderungen im Vergleich: vA...vB`)
   - `larknafets/ha-addons` `nebenkostenrechner/config.yaml` shows `version: "X.Y.Z"` (mirrored by the bot)
   - image `ghcr.io/larknafets/nebenkostenrechner:X.Y.Z` exists, `Validate Add-ons` in `ha-addons` is green
7. Remind the user: back up the database before updating. Never claim a check ran that did not.

A wrong tag that is not pushed yet is deleted locally (`git tag -d`). A pushed tag is never moved without asking.

## Channels: release and nightly

- **Release** is a tag `v*`: `release.yml` builds the image and creates the GitHub release from `CHANGELOG.md`, `ha-addon.yml` mirrors the version into `larknafets/ha-addons`, folder `nebenkostenrechner`.
- **Nightly** is `main` from the last night: `nightly.yml` (02:17 UTC, skipped without a commit in the last 24 hours, or started by hand for any branch) pushes the image tags `nightly` and `nightly.YYYYMMDD-<sha7>` and writes the dated version into `larknafets/ha-addons`, folder `nebenkostenrechner-nightly`. Only the newest 7 dated images are kept.
- No separate nightly or release branch: `main` is the nightly, a tag is the release.
- The nightly add-on has its own slug, its own data and the host port 8082 for the widget routes (release: 8081).

Changelog rules and release notes: `changelog.md`.
