# Persist garden-wide judgment runs

## Context and Problem Statement

Planty writes one verdict per plant, but the product also makes one garden-wide claim: whether the daily check is fresh and complete enough to say nothing needs attention.
Individual verdict timestamps cannot prove that claim.
If one plant succeeds and the remaining judgments fail, the newest verdict still looks fresh and the old digest code can report every live plant as checked.
A process crash halfway through the loop has the same problem: yesterday's complete-looking state can survive today's interrupted attempt.

The system needs to preserve useful per-plant verdict history while making completeness of the latest daily attempt durable and explicit.

## Considered Options

- Infer completeness from the newest verdict timestamp and current plant count.
- Fail the entire daily command unless every plant succeeds, without storing run state.
- Persist one garden-wide judgment run with expected, succeeded, failed, and completion state.

## Decision Outcome

Chosen: **persist one garden-wide judgment run with expected, succeeded, failed, and completion state**.

The daily job creates the run before the first model call, fixing `expected` to the plants that attempt promised to inspect.
After each plant, it immediately increments either `succeeded` or `failed`.
Only after the loop does it mark the run complete.
The newest attempt, including an unfinished one left by a process crash, controls whether Today may trust the garden-wide result.

Individual verdicts remain the source of actionable instructions and history.
A partial run does not delete or acknowledge prior actionable verdicts, but it cannot produce an all-clear.
Today receives the run counts explicitly and renders an incomplete attempt as untrusted rather than fresh simply because one verdict is recent.

Inferring from timestamps was rejected because it cannot distinguish one success from complete coverage.
Only returning a process error was rejected because command failure is ephemeral; a restart or a later API request still needs to know that the most recent attempt was incomplete.

### Consequences

Good:

- A partial model outage cannot claim the whole garden was checked.
- A hard process interruption leaves an explicitly incomplete newest run.
- `checked` describes successful judgments rather than the current number of plants.
- The iOS app and notification digest share the same completeness truth.
- Historical per-plant verdicts stay intact.

Bad, and accepted:

- The daily loop performs one small counter update per plant in addition to saving verdicts.
- `judgment_runs` stores aggregate outcomes, not one result row per plant. If Planty later needs to explain exactly which failed plants belonged to an old run, the schema will need a per-run result table.
