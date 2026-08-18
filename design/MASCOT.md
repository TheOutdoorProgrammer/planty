# The mascot

**Planty's mascot is a seal.**
Lavender, delighted, gleefully overwatering a plant that is visibly drowning.

Art lives in `design/logo/seal/`.

## How it got here

The brief started as "Patrick Star holding a potted plant".
That is Nickelodeon's character and this plugin ships under an LLC's public org, so it became an original dopey pink starfish instead: same joke, no takedown.

The starfish worked but was drawn as hand-authored SVG, and the monochrome variant rendered at 0.17% ink coverage, which is effectively a blank file.
Regenerating as illustration rather than SVG fixed that immediately.

Then the question changed from "how do we draw it" to "what animal is it".
Five were generated with an identical scene so the animal was the only variable: axolotl, pug, manatee, toad, pigeon.
Joey picked the manatee, which everyone including him calls the seal, so it is a seal.

## Open

**Open eyes versus closed eyes.**
Both variants exist for the hero and the icon, with measured scale tests at 512, 64 and 32 pixels in `design/logo/seal/COMPARISON.png`.

The recommendation is **open eyes**, on two grounds:

- The vacant, slightly-too-far-apart stare is the joke. Closed eyes read as contented rather than empty, and contentment is not the brief.
- At 32px the eye whites give the icon two high-contrast anchor points. The closed version collapses into a single purple mass.

## What the older design docs say

`design/UI-CONCEPT.md`, `design/DESIGN-NOTES.md` and `design/logo/README.md` all describe a pink starfish.
They were written before the animal changed and are otherwise still accurate, so they were left alone rather than rewritten.
`design/logo/animals/ANIMALS.md` recommends the axolotl; that was the generating agent's opinion, and Joey chose otherwise.

This file is the current answer.

## Where it may and may not appear

The seal appears in calm, success and gentle guidance states.

**It never appears beside a severe alert.**
A grinning seal next to "possible root rot" makes the product feel unserious exactly when it needs to be trusted.
There is a test in the iOS app asserting this.
