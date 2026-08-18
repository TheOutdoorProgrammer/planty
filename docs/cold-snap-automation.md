# Bring the plants in

The first thing Planty should do, and the only piece with a real deadline.

Your friend's sheet says it plainly: below 55F they come indoors, one night is survivable, and *consistent* weather below 55F will kill them.
It is the only instruction in the sheet whose stated consequence is death, it applies to five plants that belong to someone else, and it needs no sensors and nothing bought.

## Why the forecast, not the thermometer

The obvious version triggers on outdoor temperature crossing 55F. That version is useless.

By the time it is 54F outside it is already dark, the plants have already been cold for hours, and the person who has to carry five pots indoors is asleep.
An alert that fires at the moment of harm is not a warning, it is a postmortem.

So the trigger is the **forecast overnight low**, checked in the afternoon while there is still daylight and a conscious human.
`weather.nws_home` already publishes it. NWS is the better of the two weather entities here for this, because its overnight lows are a real forecast product rather than an interpolation.

## The design

**Trigger:** time, once daily, mid to late afternoon. Early enough to act, late enough that the forecast has settled.

**Condition:** tonight's forecast low is at or below the threshold.

**Threshold: 58F, not 55F.** Three reasons to build in margin:

- A forecast low is a regional number, and a front porch at 3am is usually colder than the airport the forecast came from
- Forecasts move, and they move down as often as up
- The cost of a false positive is carrying plants inside unnecessarily. The cost of a false negative is killing someone else's plants. These are not remotely symmetrical, so bias hard toward the cheap mistake.

**Action:** actionable notification naming the plants, the forecast low, and the time. Not a generic "it's cold."

**Escalation:** unacknowledged an hour before sunset, announce it on the house speakers via the existing `script.announce`. Unacknowledged after dark, repeat. This is the rare case that earns escalation: it is other people's plants, the window closes at sunset, and there is no second chance the next morning.

**Acknowledgement:** the notification carries a "brought them in" action that sets an `input_boolean`. That boolean is what stops the escalation, and it is also what tells the reverse automation there is something outside to bring back.

## The half nobody builds

An automation that only tells you to bring plants *in* leaves five tropical plants sitting in a dark room in August, which over a week is its own way of killing them.

So there is a second automation: once the boolean says they are inside, and the forecast shows a daytime high comfortably above the threshold with no cold night following, **tell you to put them back out.**

This is the half people forget, and it is the half that quietly does the damage, because bringing plants in feels like the responsible act and nobody notices the plants slowly declining indoors afterwards.

## Runaway safety

This one is a notification, not a valve, so it cannot flood anything.
But the failure mode still matters: a repeat loop that keeps announcing after the plants are already inside will train you to ignore announcements, and the announcement channel is shared with things that matter more.

So the escalation is bounded by count, not only by the acknowledgement boolean, and the boolean is `restored` across restarts.

That restart detail is the same trap that cost the water: Home Assistant restarts twice a day here, and anything depending on `for:` duration or on unrestored state silently resets. Applies to notification loops exactly as much as to valves.

## What it does not do

It does not move the plants. It does not know whether they are actually outside beyond what you told it. It does not distinguish which plants are on the porch versus already indoors, until the plant registry exists and carries a location.

Once the registry lands, this gets better: it names the specific plants that are outside, and it stops nagging about the ones already in.

## Open

- Confirm the porch is the coldest location, and whether it is covered. A covered porch runs a few degrees warmer than open air and holds heat longer
- Whether the sequoia sprout has a different threshold. Giant sequoia is a mountain tree and is far more cold tolerant than the tropicals, so it may not need to come in at all. Worth asking the owner rather than assuming, since the answer changes how many pots get carried
