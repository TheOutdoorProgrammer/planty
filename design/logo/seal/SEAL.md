# Planty seal

## Decision: closed eyes

**Joey picked closed on 2026-08-18, over the recommendation below.** His reason: "open looks like a maniac".

That is the recommendation's own argument landing badly.
The case for open was that pinprick pupils and oversized whites read as *vacant confidence*, which is the overwatering joke.
On a real screen they read as unhinged instead, which is a different feeling and not a warmer one.
The gap between "delighted cluelessness" and "maniac" turns out to be smaller than the analysis assumed, and it is not a gap you can measure in pixel counts, which is what the analysis had to work with.

Closed ships. The app already carried the closed-eyed artwork, so nothing needed redrawing.

The original recommendation is kept below unchanged, because the reasoning is still worth reading and the pixel measurements still describe the two files.

## Recommendation, not taken

Ship the open-eyed version.
The closed eyes are warm and charming, but they make the seal look happily competent or celebratory.
The open eyes change the read immediately: the pinprick pupils, oversized whites, and slightly excessive spacing create the vacant confidence that makes the overwatering joke land.
The open version also holds a more distinctive expression at small sizes, where the closed version becomes a familiar generic happy mascot.

The closed-eyed hero is the incumbent artwork unchanged, so the comparison does not handicap it with a redraw.
The open-eyed hero preserves the scene and swaps the emotional signal from contentment to delighted cluelessness.

## Hero versus icon

The hero is a narrative illustration.
It keeps the watering can, torrent, overflowing pot, puddle, drooping foliage, attached dying leaf, and fallen leaf because the accumulating evidence of disaster is the joke.

The icon is a separate head-and-shoulders design, not a crop.
It removes the pot, plant, watering can, puddle, loose droplets, whiskers, and other fine detail.
It uses a large centered silhouette, thick dark outlines, broad lavender and cyan regions, an oversized mouth, and one small cyan water accent.
Those choices preserve the mascot's identity when the available canvas is only 32 pixels wide.

Both wordmarks use the recommended open-eyed icon.
The dark wordmark is rendered on `#282a36` with `#f8f8f2` text.
The light wordmark is rendered on white with `#282a36` text, so the text remains fully visible.

## Raster verification

The source icons are 1254 by 1254 pixel RGBA PNGs.
Each icon was rasterised to exactly 32 by 32 pixels with Lanczos downsampling, then enlarged to exactly 256 by 256 pixels with point filtering.
The point-filtered views are included in `COMPARISON.png`, alongside actual-size 32, 64, and 512 pixel renders.

At a 50% alpha threshold, both 32 pixel variants have a visible bounding box of 32 by 27 pixels at offset `+0+3`.
Each silhouette contains 681 visible pixels out of 1,024, or 66.5% canvas coverage.
The matching silhouette measurements confirm that the eye comparison is not being distorted by different framing.

The open-eyed 32 pixel raster retains 26 light eye pixels, 166 cyan pixels, 82 dark outline-and-mouth pixels, and 14 pink tongue pixels.
The closed-eyed 32 pixel raster retains 158 cyan pixels, 86 dark outline-and-mouth pixels, and the same 14 pink tongue pixels.
The open and closed 32 pixel rasters have a normalized RMSE of 0.077, which is enough to make the eye treatment visibly different without changing the underlying mascot.

I visually inspected both 256 pixel nearest-neighbor enlargements after measuring them.
In both, the oval head, shoulders, muzzle, open mouth, and tongue remain separate distinguishable shapes rather than merging into a blob.
The open version retains two obvious white eyes and pinprick pupils; the closed version retains two readable dark arcs.

## Production notes

The open hero was produced as a precise eye edit against the incumbent art, with the original alpha silhouette restored after generation.
The icon variants were generated as simplified mascot marks and keyed to RGBA locally so they can be placed on either wordmark background.
The comparison sheet uses the actual output files rather than screenshots or stand-ins.
