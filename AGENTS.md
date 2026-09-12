## Project

Web app for monthly utility cost billing for a two-family house with heat pump and PV system. A Home Assistant Add-on wrapping this app lives at https://github.com/larknafets/ha-addons, subfolder `nebenkostenrechner` - also accessible locally at `../ha-addons`.

## Agent skills

- **Issue tracker**: GitHub Issues via `gh` CLI (larknafets/nebenkostenrechner). See `docs/agents/issue-tracker.md`.
- **Domain docs**: Single-context: root `CONTEXT.md` + `docs/adr/`. See `docs/agents/domain.md`.

## Plan mode

- Make the plan extremely concise. Sacrifice grammar for the sake of concision.
- At the end of each plan, give me a list of unresolved questions to answer, if any.

## Writing style

- No em dashes (—) in GitHub issue titles, bodies, comments, commit messages, and committed files (e.g. prototype HTML). Use commas, colons, or regular hyphens (" - ").

## Git rules

- **Author identity**: Always use the name and email already configured for the GitHub account in use (`git config user.name` / `user.email`, or the target repo's existing committer identity). Never assume, guess, or substitute a different identity (e.g. a system/session email) for commit author or committer.
- **Commit messages**: `docs/agents/git-commit-messages.md`
- **Branch naming**: `docs/agents/git-branches.md`
- **Releases and versioning**: `docs/agents/git-releases.md`

## Language convention

`docs/agents/language-convention.md`
