# Changelog and release notes

- `.changelog/unreleased.md` collects the entries of the next release, `CHANGELOG.md` is the running history (newest release first). The rules for the wording are the comment at the top of `unreleased.md`.
- Add the line in the **same commit** as the change, for anything a user or the operator of the Home Assistant app can see or feel. German, in the words of the user interface, from the user's side, with `(#nr)` if an issue drove it.
- Sections, always in this order, none removed: `## Neue Funktionen`, `## Verbesserungen`, `## Fehlerkorrekturen`, `## Wartung`.
- **Wartung** is what an operator must know when updating: Go version, base image, environment variables, ports, data paths, migrations, security updates. Dependabot updates are **one** line ("Aktualisierung der Abhängigkeiten"), added at the release if Dependabot commits landed since the last tag (`git log <last tag>..main --author=dependabot`), not per update. A `refactor` since the last tag gives **one** more line in Wartung, added at the release too: "Interne Aufräumarbeiten am Code ohne sichtbare Änderung" (`git log <last tag>..main --grep "^refactor"`), never one line per refactoring.
- No line for `docs`, `test`, `style`, `ci`, scripts, hooks, agent rules or a pipeline repair nobody notices. Refactorings only as the one collective line above.
- At the release, `CHANGELOG.md` gets the new section with the entries (sections as `###`, empty sections left out), `unreleased.md` is emptied but keeps the comment and all four headings.
- The history can be revised later at any time: `CHANGELOG.md` is a plain file, the text of an old GitHub release is changed with `gh release edit <tag> --notes-file <file>`. The `ha-addons` changelog is rebuilt from all releases at every release, so it follows.

## Release notes

`release.yml` creates the GitHub release with `gh release create`. The text is the section of this version from `CHANGELOG.md` (`###` becomes `##`) plus the German footer `Alle Änderungen im Vergleich: [vA...vB](compare link)`. The Home Assistant changelog in `ha-addons` is built from those texts by `ha-addon.yml`. There are no binaries and no checksums, the image from the `Dockerfile` is the artifact.
