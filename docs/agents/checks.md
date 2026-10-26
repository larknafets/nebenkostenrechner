# Checks before a commit

- `scripts/check` runs the same steps as the CI job `test` (gofmt, em dash scan and merge conflict marker scan of the tracked files, `go vet`, golangci-lint, `go test`, `go build`) plus `actionlint` for the workflows. `scripts/check --vuln` adds `govulncheck`, which CI runs nightly only. It stops at the first failure and fails loudly if a tool is missing.

- `scripts/screenshots` regenerates `docs/screenshots` (linked from the README) from the demo data with headless Chrome, needs Node, Chrome and `cwebp`. Run it after a visible UI change and commit the images.
- `scripts/smoke` starts the app on a temp database and fetches the main pages, a page passes with status 200 and a closing `</html>` (html/template flushes the 200 before a mid-render error). Run it after a change to a handler or template, CI does not run it.
- The pre-push hook also refuses a push to `main` that changes the Go version in `go.mod`, the `Dockerfile` or `.github/workflows/` (`scripts/pr-only-paths`): those go through a PR (`git-workflow.md`).
- The commit-msg hook `.githooks/commit-msg` (same `core.hooksPath`, so worktrees get it too) rejects a commit whose subject is not `<type>(<scope>): <summary>`, is longer than 72 characters, or whose message contains an em dash or a common ASCII spelling of an Umlaut (`geaendert`, `Rueckgabewert`, a word list of stems, not a general pattern). The rules behind it: `git-commit-messages.md`.
- Tools: `brew install golangci-lint actionlint govulncheck`. CI pins golangci-lint in `.github/workflows/ci.yml` (`version:`), `scripts/check` warns when the local version differs. The pin does not follow Dependabot: raise it together with every Go version bump, an older linter cannot read the export data of a newer Go.
- Run it before every commit. The pre-push hook `.githooks/pre-push` runs it before every push, enable it once per clone with `git config core.hooksPath .githooks`. Skip it only in an emergency (`--no-verify`).
- Before opening a PR, review the diff locally with `/review` instead of waiting for a review bot on the PR. Sourcery on GitHub is optional and not a required check (branch protection only requires `test`).
- `actionlint` finds syntax and expression errors in `.github/workflows/`, not wrong behavior. See `git-workflow.md` for when a workflow change needs a PR.
- A test must not depend on the outside world (the live GitHub API, the clock, the newest release). `TestFooterVersion_UpdateDotHidden` once used `v0.13.0` against the real latest release and broke with `v0.13.1`.
