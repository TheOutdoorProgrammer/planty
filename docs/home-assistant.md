# The Home Assistant side

Planty does the judging. A few things belong in Home Assistant instead, either because they are simpler as a schedule or because automating them properly would make things worse.

## The airflow fan

One fan on a Shelly plug, on a fixed schedule. It does two jobs at once.

**For the mushroom kit it is the variable that decides whether the kit works at all.** A sealed kit accumulates CO2, the mushrooms stretch upward hunting air, and you get long thin stems with tiny deformed caps. That gets misread as dryness, which leads to more misting, which causes bacterial blotch. Fresh air exchange fixes both at once: it clears the CO2 and dries the cap surfaces enough that blotch cannot take hold.

**For indoor tomatoes it is what makes fruit happen.** Tomato flowers are self-fertile but need physical agitation to shake pollen loose. Outdoors that is wind and bees. In a still cabinet the flowers yellow and drop and the yield is zero, while the plant looks perfectly healthy the whole time.

```yaml
alias: Planty - greenhouse airflow
description: >-
  Fresh air exchange for the mushroom kit, and pollination agitation for the
  tomatoes. Both want the same thing: moving air, several times a day.
triggers:
  - trigger: time_pattern
    hours: /3
conditions:
  - condition: time
    after: "06:00:00"
    before: "22:00:00"
actions:
  - action: switch.turn_on
    target:
      entity_id: switch.greenhouse_fan
  - delay: "00:15:00"
  - action: switch.turn_off
    target:
      entity_id: switch.greenhouse_fan
mode: single
```

`mode: single` matters: without it a restart mid-run can stack overlapping instances and leave the fan on indefinitely.

## The mushroom reminder

Deliberately a reminder and nothing more.

Misting is a **visual** decision, not a numeric one. The correct trigger is "the surface looks dry and there is no water beading on the caps." An RH-threshold automation cannot see that, so it over-mists, and over-misting is precisely the bacterial blotch failure mode. Automating this actively causes the disease it looks like it should prevent.

The daily human look **is** the control loop. Pinning, leggy stems, yellowing spines and the first blotch lesion are all eyeball findings that need a response within a day or two.

```yaml
alias: Planty - mushroom kit check
description: >-
  A look, not an automation. Misting is a visual call; an RH threshold over
  mists and causes bacterial blotch.
triggers:
  - trigger: time
    at: "09:00:00"
conditions:
  - condition: state
    entity_id: input_boolean.mushroom_kit_active
    state: "on"
actions:
  - action: notify.notify
    data:
      title: Mushroom kit
      message: >-
        Look at the kit. Surface dry with no beading on the caps means mist
        around it, not at it. Stems going long and thin means more air, not
        more water.
mode: single
```

The `input_boolean` exists so the reminder stops when the block is spent, rather than becoming noise you learn to swipe away. A kit runs four to eight weeks and yields two or three flushes.

## What is deliberately not here

**No RH-triggered misting.** See above. This is the one automation that would make things worse.

**No moisture-triggered watering yet.** Nothing may drive a pump until its probe has recorded dry and wet baselines. An uncalibrated reading is not evidence, and a confident wrong watering decision is worse than no decision.

**No `for:`-duration safety caps on anything that moves water.** A `for:` trigger re-stamps `last_changed` on every restart, so the clock restarts at zero and the restored state is never seen as a transition into it. That is how a valve here ran 14h33m past a 45 minute cap. Anything gating a pump needs a `time_pattern` backstop, a `homeassistant`/`event: start` trigger to catch a runaway after a restart, and the duration as a **condition** rather than a trigger.

## What Planty needs from Home Assistant

A long-lived token that can:

- read sensor states, for `planty ingest`
- read the weather forecast, for `planty cold`
- call `notify`, for the digest and the cold warning
- call `script.announce`, which is reserved for a plant genuinely at risk

## The LetPot

Its internal model is DI-3, and Home Assistant core does not support it. `HSTEP/letpot2.0-home-assistant` is a custom component that does, giving a pump switch and an "actively pumping" binary sensor.

That component's own README says the valve's built-in offline schedules are unreliable and recommends driving watering from Home Assistant instead. Doing that is what makes the closed loop possible: sensor reads dry, pump runs, the pump confirms it ran, and the soil sensor confirms water actually arrived. When the pump ran and the soil did not change, that dripper is clogged.

It is cloud-dependent, so no internet means no watering.
