# Commit messages

Write commit messages terse and exact. Conventional Commits format. No fluff. Why over what.

## Rules

### Splitting commits

- One commit = one logical change. Split when a diff mixes unrelated concerns (e.g. tooling setup vs. a doc/convention change), even if done in the same session.
- Each commit must build/pass on its own where feasible - don't split a change so a part is broken mid-history.
- Don't split further than one logical unit (e.g. don't split one feature's src + its own tests into two commits).

### Subject line

- `<type>(<scope>): <imperative summary>`
- Types, always lower case:
  - `feat`: a new feature
  - `fix`: a bug fix
  - `chore`: changes unrelated to a fix or feature, not touching src or test files (e.g. updating dependencies)
  - `refactor`: refactored code that neither fixes a bug nor adds a feature
  - `docs`: updates to documentation such as the README or other markdown files
  - `style`: changes that don't affect code meaning (whitespace, missing semicolons, etc.)
  - `test`: new or corrected tests
  - `perf`: performance improvements
  - `ci`: continuous integration, files under `.github/workflows/` (when/how the pipeline runs)
  - `build`: the release/packaging tool's own config, e.g. `.goreleaser.yaml`, `Dockerfile` (how the artifact is produced), or external dependencies
  - `revert`: reverts a previous commit
- A bug in a `ci` or `build` file goes under that type, not `fix`.
- The `<scope>` is always lower case and can be empty (e.g. if the change is a global or difficult to assign to a single component), in which case the parentheses are omitted.
- Component name is the default scope. Only when no component fits (global/cross-cutting change) and an issue or discussion number exists, use `#<nr>` as scope instead of leaving it empty. Never combine component and issue number in one scope.
- Issue/discussion number in scope never replaces the body reference (`Closes #42`, `Refs #17`) - both stay.
- Imperative mood: "add", "fix", "remove" - not "added", "adds", "adding"
- ≤50 chars when possible, hard cap 72
- No trailing period

### Body (only if needed)

- Skip entirely when subject is self-explanatory
- Add body only for: non-obvious *why*, breaking changes, migration notes, linked issues
- Wrap at 72 chars
- Bullets `-` not `*`
- Reference issues/PRs/discussions at end: `Closes #42`, `Refs #17`

### What NEVER goes in

- "This commit does X", "I", "we", "now", "currently" - the diff says what
- "As requested by..."
- "Generated with Claude Code" or any AI attribution
- Emoji
- Restating the file name when scope already says it

## Examples

Diff: new endpoint for user profile with body explaining the why
- ❌ "feat: Add a new endpoint to get user profile information from the database"
- ✅
  ```
  feat(api): Add GET /users/:id/profile

  Mobile client needs profile data without the full user payload
  to reduce LTE bandwidth on cold-launch screens.

  Closes #128
  ```

Diff: global change with no fitting component, tracked in issue #42
- ✅
  ```
  refactor(#42): switch logging to structured JSON

  Refs #42
  ```

Diff: breaking API change
- ✅
  ```
  feat(api)!: Rename /v1/orders to /v1/checkout

  BREAKING CHANGE: clients on /v1/orders must migrate to /v1/checkout
  before 2026-06-01. Old route returns 410 after that date.
  ```

## Auto-Clarity

Always include body for: breaking changes, security fixes, data migrations, anything reverting a prior commit. Never compress these into subject-only - future debuggers need the context.