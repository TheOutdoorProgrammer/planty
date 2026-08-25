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

## Closed

**Open eyes versus closed eyes: settled, closed.**
Both variants exist for the hero and the icon, with measured scale tests at 512, 64 and 32 pixels in `design/logo/seal/COMPARISON.png`.

Joey chose **closed** on 2026-08-18, against the recommendation, because open "looks like a maniac".
The case for open was that the vacant, slightly-too-far-apart stare *is* the joke, and that at 32px the eye whites give the icon two high-contrast anchor points where closed collapses into one purple mass.
The second point is measurably true and the first turned out not to be: the intended vacancy reads as unhinged on a real screen, and an app that is supposed to be reassuring about your plants cannot open with an unhinged face.
`design/logo/seal/SEAL.md` keeps the full original argument.

## Historical exploration

The current UI and design-decision documents use the seal.
`design/logo/`, `design/logo/raster/`, and `design/logo/animals/` preserve the earlier starfish and animal-selection work as design history rather than current brand guidance.
`design/logo/animals/ANIMALS.md` recommends the axolotl; that was the generating agent's opinion, and Joey chose otherwise.

## Where it may and may not appear

The seal appears in calm, success and gentle guidance states.

**It never appears beside a severe alert.**
A grinning seal next to "possible root rot" makes the product feel unserious exactly when it needs to be trusted.
There is a test in the iOS app asserting this.
