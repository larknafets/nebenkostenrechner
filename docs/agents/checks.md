# Checks before a commit

- `scripts/check` runs the same steps as the CI job `test` (gofmt, em dash scan of the tracked files, `go vet`, golangci-lint, `go test`, `go build`) plus `actionlint` for the workflows. `scripts/check --vuln` adds `govulncheck`, which CI runs nightly only. It stops at the first failure and fails loudly if a tool is missing.
- The commit-msg hook `.githooks/commit-msg` (same `core.hooksPath`, so worktrees get it too) rejects a commit whose subject is not `<type>(<scope>): <summary>`, is longer than 72 characters, or whose message contains an em dash. The rules behind it: `git-commit-messages.md`.
- Tools: `brew install golangci-lint actionlint govulncheck`. CI pins golangci-lint `v2.13.2`, Homebrew may be newer: if CI reports something the local run did not, check the version first.
- Run it before every commit. The pre-push hook `.githooks/pre-push` runs it before every push, enable it once per clone with `git config core.hooksPath .githooks`. Skip it only in an emergency (`--no-verify`).
- Before opening a PR, review the diff locally with `/review` instead of waiting for a review bot on the PR. Sourcery on GitHub is optional and not a required check (branch protection only requires `test`).
- `actionlint` finds syntax and expression errors in `.github/workflows/`, not wrong behavior. See `git-workflow.md` for when a workflow change needs a PR.
- A test must not depend on the outside world (the live GitHub API, the clock, the newest release). `TestFooterVersion_UpdateDotHidden` once used `v0.13.0` against the real latest release and broke with `v0.13.1`.
