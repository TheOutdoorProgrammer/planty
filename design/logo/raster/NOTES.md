# Planty raster logo explorations

> **Historical exploration.** These starfish concepts are superseded by the seal in `design/logo/seal/`.

These concepts were generated as original raster illustrations using the built-in image-generation workflow, then assembled into comparison assets locally.
Every concept uses the same central joke: the starfish is enormously proud while the plant is plainly having a terrible time.
The contact sheet is ordered left to right, top to bottom from V1 through V6.

## Variants

### `planty-v1-upside-down-pot.png`

The starfish presents a nearly inverted pot while soil escapes and a brown leaf hangs on for dear life.
This lands the joke immediately, and the raised-pot silhouette is strong.
However, the wide eyes and open mouth drift too close to the Patrick-style energy that the brief explicitly asked us to avoid.
I would not ship this one without a substantial facial redesign.

### `planty-v2-overwatering.png`

The starfish floods a broken, wilted plant and gives itself an enthusiastic thumbs-up.
This is the funniest standalone illustration because the cause and effect are unmistakable even without context.
It is also the busiest concept: the water, can, pot, five arms, and dropped leaf collapse together at app-icon sizes.
Best use would be a larger repository illustration, not the primary mark.

### `planty-v3-five-arm-pot-wrestling.png`

All five arms participate in an awkward pot-clutching disaster while soil spills and three leaves show three stages of surrender.
The unusual eye proportions and tangled pose make the character feel more original than V1.
The pot and arms overlap too heavily for a clean tiny silhouette, and the expression reads more cute than magnificently stupid.
This one partially succeeds, but it is not a finalist.

### `planty-v4-trophy-pot.png`

The starfish hoists the failing plant like a championship trophy while a dead leaf falls off in real time.
This is the strongest primary mark.
The pose is simple, the joke remains visible, and the silhouette survives down to 32 pixels better than the other variants.
It also has the clearest visual hierarchy: delighted idiot first, doomed plant second, falling leaf as the delayed punchline.
This variant drives `SMALL-SIZE-TEST.png` and the light-background wordmark.

### `planty-v5-too-much-love.png`

The starfish hugs the pot hard enough to crack it, blissfully mistaking destruction for affection.
This is the strongest character illustration and the best expression in the set.
The joke is warm, stupid, and specific rather than merely showing a mascot beside an unhealthy plant.
Its overlapping arms, dirt, crack, and leaves become visually noisy at 32 pixels, so I prefer it for headers and larger placements rather than the app icon.
This variant drives the dark-background wordmark.

### `planty-v6-pot-hat-parade.png`

The starfish wears the pot as a crown and parades while the plant sheds leaves behind it.
This is the most kinetic and most distinct interpretation.
The tall dancing silhouette is charming, but the pot hides the top of the body and the plant becomes a complicated crown-shaped mass when reduced.
It works as a sticker or secondary illustration, not as the core logo.

## Wordmark lockups

- `planty-wordmark-dark-too-much-love.png` pairs V5 with an off-white rounded wordmark on a Dracula-dark background.
- `planty-wordmark-light-trophy-pot.png` pairs V4 with a Dracula-dark rounded wordmark on an off-white background.

Both wordmarks render `Planty` correctly and retain the damaged-plant gag.
The dark lockup has more personality; the light lockup is cleaner and more immediately logo-like.

## Recommendation

Use V4 as the app icon and compact mark.
Use the V5 dark lockup as the repository header when the extra detail has room to breathe.
Keep V2 as the larger comic illustration because it gets the biggest laugh, even though it is not a good tiny logo.

V1 is the clearest miss because of resemblance risk.
V3 and V6 are worthwhile explorations but lose too much structure at small sizes.

## Comparison assets

- `CONTACT-SHEET.png` tiles all six square variants on Dracula dark for side-by-side comparison.
- `SMALL-SIZE-TEST.png` shows V4 at native 64-pixel and 32-pixel sizes without sharpening tricks.

## Prompt set

All six generations used the `logo-brand` prompt class with a square, centered, high-contrast, vector-like 2D illustration brief; the shared palette was Dracula Classic, and every prompt explicitly required a delighted original pink starfish, five readable arms, a visibly failing plant, no text, no clothing, no body spots, no copyrighted-character likeness, no photorealism, no 3D rendering, and no watermark.
The six distinct scene prompts were: nearly upside-down pot, destructive overwatering, five-arm pot wrestling, trophy presentation with a falling leaf, pot-crushing hug, and pot-as-hat parade.
The two wordmark edits preserved the selected generated characters and added the exact text `Planty` in a rounded, chunky horizontal lockup for dark and light backgrounds respectively.
