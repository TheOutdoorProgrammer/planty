# Rate toxicity per audience, and name who graded it

## Context and Problem Statement

Planty should be able to say whether a plant will hurt a cat, a dog or a person, and why.

This is the first field in the app where being confidently wrong hurts something living.
Every other mistake it can make is recoverable: a plant gets watered late, a verdict is unhelpful, a reminder fires at the wrong hour.
Telling somebody a plant is safe for their cat when it is an Easter lily is not in that category.

The obvious shape (one severity per plant, on a scale invented for the purpose) is wrong twice over, and the research that produced this record is what made both visible.

## Considered Options

- **One severity per plant.** Simplest, and what most plant apps do.
- **A five-level scale of `unknown | safe | irritant | toxic | severe`.** The first proposal here.
- **Per-audience ratings on a four-level scale defined by owner action, plus provenance fields.** What was built.
- **Per-audience, per-route ratings.** The most faithful model, rejected below.

## Decision Outcome

Per-audience ratings on `unknown | safe | mild | moderate | severe`, with the levels defined by what the owner should do rather than by pharmacology, plus the fields that make a rating checkable: the botanical name it was resolved against, the toxic principle behind it, and who did the grading.

### Why the audiences are separate

Because they genuinely diverge, and the divergence is concentrated in exactly the cases that kill animals.
An Easter lily is acute renal failure in a cat, frequently fatal, and a mild stomach upset in a dog; Merck records it as not reported toxic to other species at all.
Alliums differ by a quantified factor of three to six between cats and dogs.
Aloe is human food and pet-toxic in the same leaf.

One flag for all three either panics dog owners or kills cats, and there is no way to pick which.

The counter-evidence is worth recording too: the entire Araceae family (pothos, philodendron, monstera, ZZ, peace lily, anthurium, dieffenbachia) is insoluble calcium oxalate and behaves identically in all three audiences.
That is most of a typical collection, so most rows will not diverge, which is precisely why divergence has to be reported loudly rather than left for someone to notice across three chips.

### Why the scale changed

`irritant` is not a point on the same axis as `toxic`.
It names a mechanism, and an irritant plant *is* toxic, as is a severe one, so the five levels could not be ordered at all.
Replacing it with `mild` puts it where it belongs, and defining each level by what the owner does (nothing, watch it, ring a vet, go now) gives boundaries the app can actually defend, since each one changes a decision.

### Why `basis` exists, and it is the important field

The primary source is binary.
The ASPCA publishes "Toxic to Cats" or "Non-Toxic to Cats" and nothing between.
The Pet Poison Helpline declines to rate plants at all, on the grounds that dose, body weight and the individual animal decide.
Merck organises houseplants by toxic principle and treats severity as a consequence of principle, species and quantity.
NC State is the only source with an ordinal severity field, publishes no calibration for what its levels mean, and is a human-facing extension service whose ratings must not be read across to pets.

So a four-level ordinal cannot be populated from the sources without inventing the middle.
Every `moderate` is a clinical judgement no cited source actually made.
`basis` makes that visible in the data rather than leaving it as something we knew for a month and then forgot: `source` means the reference stated that level, `derived` means Planty graded it from the principle.

`Valid` enforces the matching rule: anything rated moderate or severe must name a toxic principle.
A rating that cannot say what mechanism it rests on was guessed, and a guess here is worse than admitting ignorance.

### Why `identified_as` matters more than the rating

"Lily" is at least six unrelated plants spanning three mechanisms and the full range from mild to fatal, and the two that kill cats are in different families from each other, so plant family is not a safety heuristic either.
Taxonomic renames make it worse: *Sansevieria* became *Dracaena*, and a lookup under the old name silently returns nothing, which reads as "no hazard found".

A toxicity rating keyed on a common name is therefore not evidence of anything.
Recording the botanical name the rating was actually resolved against is what makes it checkable later, and it is the field most likely to reveal that a row is wrong.

### Why `unknown` stays inside the enum

The research argued for pulling it out into a nullable column so it could never sort adjacent to `safe`.
The concern is right and the remedy is different here: `rank` places unknown *above* safe deliberately, so sorting by risk surfaces the plants nobody has checked instead of burying them, and the UI is required to render it as visibly unresolved rather than as a calm neutral.

Keeping it in the enum means the Go zero value of a `Toxicity` is honest, since an untouched plant reads as unknown rather than as an empty string nobody has decided the meaning of, and it avoids a `*Harm` at every call site that could be nil-dereferenced or silently skipped.

### What is stored, and why it is generated

The record is one JSONB document, because most of it is prose nobody filters on.
The three per-audience ratings are `GENERATED ALWAYS AS` columns derived from that document, with a `CHECK` constraint over them.

This buys three things at once: "what in this house can hurt the cat" is an indexed query over real columns; a rating can never drift from the reasoning that justifies it, because there is only one copy; and a typo in the JSON fails the write instead of becoming a silent fourth state that renders as neither dangerous nor safe.

Verified against Postgres 18: a new row defaults to `unknown` on all three, a written document propagates, a misspelt level is rejected by the constraint and leaves the previous value intact, and the generated columns cannot be written directly.

## Consequences

Good:

- The lily case is representable, which was the whole point.
- A wrong entry can be re-checked at the source that made the claim, because the record says which host, which botanical name, and which mechanism.
- The schema admits which gradations Planty invented, so nobody has to remember.
- Every existing plant migrated to "nobody has checked", never to "safe".

Bad:

- **Severity is per audience, not per route.** Euphorbia sap damages eyes without anyone eating it, and cultivated oyster mushrooms are supermarket food that documentably cause hypersensitivity pneumonitis when their spores are inhaled indoors. The faithful model is `(audience, route, severity)`, which is twelve ratings per plant that nobody would ever fill in. The compromise is that severity means the worst plausible household route, with `routes` listing which are implicated and `notes` explaining when the route changes the answer. This is a real loss of fidelity and the most likely thing to want revisiting.
- A generic "what to do if eaten" field was deliberately not added, because it collapses to the same paragraph for nearly every plant and is better templated from the severity. `first_aid` exists only as a sparse override for the small set where the obvious advice is actively wrong.
- "Non-toxic" does not mean harmless, and the app inherits that ambiguity from its sources. The ASPCA states plainly that its list is not all-inclusive and that any plant may cause vomiting, so absence of a hazard is not evidence of safety.
