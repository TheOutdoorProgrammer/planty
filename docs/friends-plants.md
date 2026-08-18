# The friend's plants

Ground truth from the owner, received 2026-08-18, while he is on sabbatical.
This is the highest-authority source about these five plants.
Where Planty's guidance disagrees with this document, this document wins.

## The plants

| Plant | Water | Notes from the owner |
| --- | --- | --- |
| **Peace lilies** (several) | **Very little** | The leafy ones with white and some green flowers. Called out as *very delicate*. Will bleach and crisp up with too much sun |
| **Bonsai** | Not specified | "Wild and seems to be okay in any situation." The hardiest of the group. If it crisps or starts dying, move it away from sun |
| **Vines** | Once every 3 days or less | |
| **Fern** | Once every 3 days or less | Hard to water normally. Lift the arms of the plant and pour directly underneath, because the foliage is so thick |
| **Sequoia sprout** | **Regular water, but not a lot at a time** | The only one asked for *consistent* moisture |

## The rules

**Light: bright indirect, all of them.** Somewhere with natural light but no sun hitting them directly. The porch qualifies, or a room the sun does not beat down into.

**The soil test, which overrides any schedule:** touch the top half inch to inch of soil. If it is wet, they do not need water. The owner gave this as the rule of thumb, so it is the arbiter whenever a schedule and the soil disagree.

**Below 55F they come indoors.** A single night, or maybe a day, is survivable as long as it is not freezing. *Consistent* weather below 55F will kill them. This is the only instruction in the sheet with death as the stated consequence.

**Equipment:** a watering can and a spray bottle are already on the porch.

**Placement:** currently all on the front porch. Explicitly free to move anywhere.

**The owner is reachable and asked to be asked.** Twice. Any real uncertainty goes to him rather than into a guess.

## What this means for Planty

### The two water profiles are opposites

The sequoia sprout wants **consistent** moisture.
The peace lilies want **very little** water and are the most delicate thing here.

They must never share a LetPot dripper line.
One pump waters everything on the line for the same duration, so a schedule that keeps the sequoia happy will steadily drown the peace lilies, and a schedule gentle enough for the lilies will let the sequoia dry out.
This is the single clearest case of the group-by-thirst rule in the whole collection.

### The 55F rule is the highest-value automation in the project

It needs no sensors, no purchases and no soil data.
`weather.nws_home` already forecasts overnight lows.
Missing one cold snap kills five plants that belong to someone else, and it is the only rule in the sheet whose stated failure mode is death.

It also has to fire on the **forecast**, not on the current temperature.
By the time it is 54F outside it is already night, the plants are already cold, and the person who needs to carry them inside is asleep.

### The soil test does not scale to the plants you cannot reach

The owner's rule of thumb requires physically touching the soil of every plant.
Several plants here sit high up and are awkward to reach daily.
Those are the ones that get soil sensors first.

Prioritise sensors by **how hard the plant is to check**, not by how much the plant is worth.
An accessible plant gets checked by hand. A plant at head height on a top shelf gets forgotten, and forgotten is how they die.

### Peace lily droop is a free early warning

Peace lilies wilt dramatically and visibly when thirsty, and recover within hours of watering.
That makes them the one plant in the group that reports its own state without hardware.

Given the owner's "very little water" instruction and that peace lilies are far more often killed by overwatering than by drought, the droop is the safer trigger than any calendar.
Do not water them on a schedule. Water them when the soil test says dry, and treat a droop as a backstop that means it was already missed.

### Open questions for the owner

- How many peace lilies, how many vines, and what species are the vines?
- What kind of bonsai? Species drives whether it needs a winter dormancy period, which matters if the sabbatical runs into winter
- How old is the sequoia sprout, and has it been through a winter yet?
- When is he back? It sets whether this is a summer problem or a winter one
