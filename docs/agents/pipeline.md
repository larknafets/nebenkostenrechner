# Pipeline: Wayfinder to epic PR

The standard order for a larger change: `/wayfinder` (map and decision tickets), `to-spec`, `to-tickets`, `/implement-spec`. Branch and PR rules: `git-workflow.md`, subagent rules: `agents.md`.

## Who starts what

- `/wayfinder` and `/implement-spec` have `disable-model-invocation`: the agent cannot start them. Give the user the full command with arguments to paste.
- `grilling`, `domain-modeling`, `to-spec`, `to-tickets` and `code-review` the agent starts itself.
- With a pipeline order from the user, do not end the turn after a resolved Wayfinder ticket. Open the next frontier ticket and put its question round right away. The rule "one ticket per session" limits the work, not the turn, and the user's answers gate the next step anyway.
- Ask before opening the PR and before switching on auto-merge.

## Context between the phases

- The state of the pipeline lives on the tracker (map, spec, tickets), so `/clear` before the next phase, in particular before `to-spec` and before `/implement-spec`. `/clear` costs nothing, `/compact` costs a call with the whole context. A fresh session loads `AGENTS.md` and this file by itself.
- Check first that no decision exists only in the conversation: it belongs in the map, spec or ticket text.
- Inside a long phase compact at about 250k tokens (25 % of a 1M window), not at 600k: every call re-reads the whole context.

## Writing the spec and the tickets

- A claim that something stays unchanged ("golden file unchanged", "tests untouched") needs proof at spec time, by reading the data or running it. Otherwise write "check first, name every change". Once the demo data held two truths and the golden file had to change.
- Tickets carry no absolute numbers that later tickets move (`schemaVersion` 2 to 3). Write "raise by one".
- Spec and ticket must not contradict each other on a helper or a name. Read both once more after writing the tickets.
- Two tickets of the same frontier that edit one package reserve their new names in the ticket text, or run one after the other. Parallel implementers pick the same name otherwise (`fixkostenHinweis` twice).

## Running the epic

- Create `epic/<name>` from `main`, one worktree branch `ticket/<nr>` per ticket.
- Before every `scripts/merge-ticket`, run `git branch --show-current` as its own step.
- After each merge `scripts/check` is green before the next implementer starts on its result.
- Review (`code-review`, two reviewers), one fix implementer, then ask for the PR.
- Documentation: see "Docs never ride in a product commit" in `git-workflow.md`.
