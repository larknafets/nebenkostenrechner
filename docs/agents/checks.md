# Checks and local tools

- `scripts/check` runs the same steps as the CI job `test` plus `actionlint` for the workflows. `scripts/check --vuln` adds `govulncheck`, which CI runs nightly only.
- `scripts/check` also fails on a tracked leftover of `sed -i` (`file.go-E`), merge or patch (`.orig`, `.rej`). On macOS write `sed -i ''`.
- `scripts/screenshots` regenerates `docs/screenshots` (linked from the README) from the demo data with headless Chrome, needs Node, Chrome and `cwebp`. Run it after a visible UI change and commit the images.
- `scripts/smoke` starts the app on a temp database and fetches the main pages, a page passes with status 200 and a closing `</html>` (html/template flushes the 200 before a mid-render error). Run it after a change to a handler or template, CI does not run it.
- `scripts/dev-server` starts the app for a manual look in the browser: a real (not demo) database with the demo data behind a login password (`dev`, `DEV_PASSWORD` changes it, port 18080, `DEV_PORT` changes it). It builds the app once, so restart it after a change to a template or Go file. Start it detached from the call (`(nohup scripts/dev-server >/tmp/dev.log 2>&1 &)`), a background job of the shell call dies with it. The demo login (`demo`) would switch to the separate demo database, so it does not show the permission rules.
- Tools: `brew install golangci-lint actionlint govulncheck`. CI pins golangci-lint in `.github/workflows/ci.yml` (`version:`), `scripts/check` warns when the local version differs. The pin does not follow Dependabot: raise it together with every Go version bump, an older linter cannot read the export data of a newer Go.
- Run it before every commit. The pre-push hook `.githooks/pre-push` runs it before every push, enable it once per clone with `git config core.hooksPath .githooks`.
- Before opening a PR for a product change run `scripts/check`, then the review before the PR (`/ponytail-review`, `/code-review` for an `epic/` branch).
- A test must not depend on the outside world (the live GitHub API, the clock, the newest release). `TestFooterVersion_UpdateDotHidden` once used `v0.13.0` against the real latest release and broke with `v0.13.1`.
