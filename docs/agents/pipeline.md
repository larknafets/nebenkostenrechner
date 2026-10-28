# Pipeline: Wayfinder to epic PR

The standard order for a larger change: `/wayfinder` (map and decision tickets), `to-spec`, `to-tickets`, `/implement-spec`. Branch and PR rules (also where documentation goes: "Docs never ride in a product commit"): `git-workflow.md`, subagent rules: `agents.md`.

## Who starts what

- `/wayfinder` and `/implement-spec` have `disable-model-invocation`: the agent cannot start them. Give the user the full command with arguments to paste.
- `grilling`, `domain-modeling`, `to-spec`, `to-tickets` and `code-review` the agent starts itself.
- With a pipeline order from the user, continue straight to the next frontier ticket after a resolved Wayfinder ticket and put its question round. The rule "one ticket per session" limits the work, not the turn, and the user's answers gate the next step anyway.

## Context between the phases

- At the end of a phase (map done, spec published, tickets published) tell the user to run `/clear` and give the full command of the next phase. The agent cannot clear the context itself.
- Before that, every decision of the phase has a place in the map, spec or ticket text (reread the answers in the chat, e.g. "Q1-3: ok"). The state of the pipeline lives on the tracker, so a fresh session loses nothing else.
- `/clear` costs nothing, `/compact` costs a call with the whole context. A fresh session loads `AGENTS.md`, which points here.
- Inside a long phase compact at about 250k tokens, not at 600k: every call re-reads the whole context.

## Writing the spec and the tickets

- A claim that something stays unchanged ("golden file unchanged", "tests untouched") needs proof at spec time, by reading the data or running it. Otherwise write "check first, name every change". Once the demo data held two truths and the golden file had to change.
- Write relative changes ("raise `schemaVersion` by one"), not absolute ones ("2 to 3") that later tickets move.
- After writing the tickets, reread spec and tickets until every helper and name agrees.
- Two tickets of the same frontier that edit one package reserve their new names in the ticket text, or run one after the other. Parallel implementers pick the same name otherwise (`fixkostenHinweis` twice).
