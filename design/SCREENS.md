# Planty screen specifications

## Shared behavior

### App shell

The app launches into Today with three tabs: Today, Snap, and Plants.
The selected tab uses pink plus a filled icon; inactive tabs use high-contrast secondary foreground.
Tab labels remain visible at all Dynamic Type sizes.

The profile button contains settings, data freshness, sensor connections, and ownership management.
None of those functions appears as a daily dashboard card.

### Shared state rules

- A **loading** state says what is happening and preserves the previous useful content when possible.
- An **empty** state explains whether setup is incomplete or there is simply no content yet.
- A **calm** state confirms that current evidence was checked and names its freshness.
- An **alert** state names one action first, then explains why.
- An **error** state never masquerades as calm and never implies the plants are in danger without evidence.

### State vocabulary

- `All good`: current evidence was evaluated and no action is recommended.
- `Watch`: there is a visible trend worth comparing later, but no action now.
- `Needs care`: one physical action is recommended now.
- `Urgent`: evidence suggests delay could cause harm.
- `Unknown`: Planty lacks current evidence; this is not equivalent to healthy or unhealthy.

## Today

### Purpose

Tell the user whether anything needs doing now and route directly into the action.

### Calm state

Exact copy:

> **You're done.**
>
> Nothing needs you right now.
>
> 8 plants checked · Updated at 8:04 AM
>
> **All quiet in the greenhouse**
>
> The next automatic check is tomorrow morning.
>
> **Take a photo anyway**

“Take a photo anyway” is secondary in visual weight but remains a full-size button.
The mascot is visible in this state.

### Action state

Exact page copy:

> **One thing. You've got this.**
>
> Everything else is okay.

Exact friend-plant card copy:

> **Maya's plant · Needs water**
>
> **Mona**
>
> Swiss cheese plant · Living room
>
> **Give it a slow drink. Stop when water reaches the tray.**
>
> The soil has stayed dry for two days, and the leaves look unchanged. This is a normal watering, not an emergency.
>
> **I'm here**
>
> **Not now**

The card has an orange action treatment and purple ownership badge.
It does not use red.

### Multiple-action state

Show at most three expanded cards, friend-owned plants first, then personal plants by urgency.
Additional actions collapse under “2 more for later today.”
Do not rank plants with numeric scores.

### Loading state

Keep the previous verdicts visible with a small top status row when cached data exists.

Exact copy with cached data:

> Checking today's evidence…
>
> Showing the last check from yesterday at 8:01 AM.

Exact copy without cached data:

> **Checking on everyone…**
>
> Planty is reading the latest photos and sensor updates.

### Empty setup state

Exact copy:

> **No plants to worry about yet.**
>
> Add the first one with a photo. If you do not know its name, Planty can help after the picture.
>
> **Add a plant**

### Stale state

Exact copy:

> **Planty needs a fresh look.**
>
> The last complete check was 3 days ago. That does not mean anything is wrong; it means Planty cannot honestly say everything is fine.
>
> **Check connections**
>
> **Take a photo**

### Error state

Exact copy:

> **Today's check did not finish.**
>
> Your saved photos and notes are still here. Planty will try again, or you can take a photo now if something looks wrong.
>
> **Try again**
>
> **Take a photo**

### Interactions and transitions

- Tap “I'm here” to open Snap with the plant locked in context and the recommended action visible after capture.
- Tap “Not now” to open a sheet offering “Remind me in 1 hour,” “Later today,” and “I already handled it.”
- “I already handled it” requires a one-tap event type and does not require text.
- Swipe actions are deliberately absent from care cards because accidental completion has real consequences.
- Pull to refresh asks the backend for current state; it does not force a new LLM analysis without explaining that cost and delay.
- When the last action is completed, the cards collapse into the copy “**That's it. Everything else is okay.**”

## Snap

### Purpose

Capture useful visual evidence and optionally record an exception with one hand.

### Ready state

Exact copy:

> **Mona · Maya's plant**
>
> Fit the whole plant in the frame.
>
> **One photo is enough.**

The plant chip is tappable and opens a bottom sheet with recent plants first, then friend and personal groups.
The shutter remains enabled when no plant is selected.

### Captured state

Exact copy:

> **What happened here?**
>
> **Watered by hand**
>
> **Repotted**
>
> **Something looks off**
>
> Add a short note (optional)
>
> **Save photo only**

The first two actions save immediately with a brief undo toast.
“Something looks off” opens Diagnosis with the photo already attached.
The note field is optional and is never focused automatically.

### Unknown-plant state

Exact copy after capture:

> **Which plant is this?**
>
> Planty thinks this might be Mona, but you should make the call.
>
> **Mona · Living room**
>
> **Pick another plant**
>
> **Add as a new plant**

The model suggestion is never silently accepted when confidence is low.

### Camera permission state

Exact copy:

> **Planty needs the camera for the useful bit.**
>
> Photos let Planty compare changes that soil and room sensors cannot see.
>
> **Allow camera access**
>
> **Choose from Photos**

### Error state

Exact copy:

> **That photo did not save.**
>
> It is still on this screen. Try again before closing so the observation is not lost.
>
> **Try saving again**
>
> **Discard photo**

Discarding requires confirmation.

### Gestures and accessibility

- Pinch to zoom and tap to focus use standard camera behavior.
- The whole lower third remains clear enough for a one-handed shutter gesture.
- Double-tapping the preview does not trigger capture; this avoids accidental duplicate records.
- VoiceOver reads the selected plant, camera framing guidance, and shutter in that order.
- The volume-button shutter should be supported in the production implementation if public APIs permit it.

### Transition

Snap uses a short freeze-frame transition into the captured state.
Saving closes to the source screen and shows “Photo added to Mona's story.”
Starting diagnosis pushes a conversation so the camera remains behind it in the navigation stack.

## Diagnosis conversation

### Purpose

Turn a current photo plus the plant's earlier evidence into a plain-language decision.

### Analyzing state

Exact copy:

> **Looking closer…**
>
> Comparing today's photo with 6 earlier photos and the recent cabinet conditions.

Show progressive stages only when they are true: uploading, comparing, and writing the answer.
Never rotate fake status phrases merely to entertain the user.

### Finding state

Exact mock response:

> **The lower yellowing has spread since July 20.**
>
> Most likely, Mona has been staying wet too long. The pattern fits overwatering better than low light.
>
> **Today: do not water it. Empty any water sitting in the tray.**
>
> I will compare another photo in 3 days.
>
> Based on 7 photos and 14 days of moisture readings.

Suggested follow-ups:

- “Show me what changed”
- “Could this be pests?”
- “What would make this urgent?”

Action buttons:

- “I emptied the tray”
- “Take another photo”

### No-concern state

Exact copy:

> **I do not see a new problem.**
>
> The pale patch is already visible in the August 11 photo and has not spread. Do not change the care plan today.
>
> If it changes, take another photo from the same side.

This phrasing is preferred to “Your plant is healthy,” which claims more than a photo can prove.

### Insufficient-evidence state

Exact copy:

> **I cannot call this yet.**
>
> The photo is useful, but the affected leaf is too far away to compare.
>
> Take one close photo of the leaf and one of the soil surface.
>
> **Take two more photos**

### Urgent state

Exact copy:

> **Keep this plant away from the others for now.**
>
> I can see webbing and clustered pale spots that could be spider mites. Move Mona away from the cabinet before taking more photos.
>
> **Show me how**
>
> **I moved it**

Urgent treatment uses red plus an explicit label and icon.
The mascot is absent.

### Offline and error states

Exact offline copy:

> **Photo saved. Diagnosis is waiting for a connection.**
>
> You can close Planty. The answer will appear here when it is ready.

Exact analysis error copy:

> **Planty could not finish this comparison.**
>
> The photo is safely in Mona's story. Try the diagnosis again, or add a note about what you saw.
>
> **Try again**
>
> **Add a note**

### Conversation behavior

- The composer accepts text, dictation, or another photo.
- Replies remain attached to this plant and observation.
- The assistant separates “what I can see,” “what it probably means,” and “what to do today.”
- A destructive or high-risk care suggestion must state uncertainty and offer escalation rather than sounding authoritative.
- Swipe back preserves the conversation and captured photo.

## Plant story

### Purpose

Show how one plant has changed, why Planty reached its current verdict, and what the user has already done.

### Current calm state

Exact copy:

> **Doing okay**
>
> New growth is steady. The lower leaves are recovering after watering less.
>
> Last compared today at 8:04 AM

Timeline copy:

> **August 18 · New growth**
>
> Two leaves are opening. The yellowing has not spread. No action needed.

> **August 11 · Holding steady**
>
> Yellowing stopped spreading after you emptied the tray on August 8.

> **July 20 · First change noticed**
>
> Lower leaves began yellowing. Moisture had stayed high for most of the week.

### Watch state

Exact copy:

> **Watching one leaf**
>
> A pale patch appeared on the newest leaf. Do not change anything today; the next photo will tell us whether it is spreading.
>
> **Take comparison photo Friday**

### No-photo state

Exact copy:

> **Mona has data, but no story yet.**
>
> Sensors can tell Planty about the pot and the room. A photo adds the part they cannot see.
>
> **Take the first photo**

### Empty-history state

Exact copy:

> **Nothing has happened yet. Nice.**
>
> Photos, care actions, and useful changes will collect here over time.

### Loading state

Show the known plant header and current cached story while newer events load.
If there is no cache, use photo-shaped and text-shaped placeholders without pulsing under Reduce Motion.

### Error state

Exact copy:

> **The newest chapter is missing.**
>
> Earlier photos and notes are still available. Planty could not load the latest events.
>
> **Try again**

### Gestures and transitions

- Tap a photo to enter a full-screen comparison viewer.
- In comparison, scrub horizontally between aligned dates; VoiceOver exposes explicit Previous and Next buttons.
- Long press on an event opens actions such as correct plant, edit note, and delete.
- Deletion confirms because observations contribute to future diagnoses.
- The floating “Take today's photo” button opens Snap with the plant selected.
- Sensor evidence is disclosed with “Why Planty thinks this,” not rendered as an always-visible graph.

## Plants library

### Purpose

Find a plant and enter its story.

### Populated state

Group sections in this order:

1. “Maya's plants”
2. “Mine”

Each row contains the latest photo, common name, species if known, room, status words, and last-photo relative date.
The status label is more prominent than the species.

### Search state

Search matches plant name, species, owner, and room.
Recent plants remain visible before typing.

Exact no-results copy:

> **No plant by that name.**
>
> Try a room or species, or add this plant with a photo.
>
> **Add a new plant**

### Empty state

Exact copy:

> **No plants yet.**
>
> Start with a photo. You can name the plant now, let Planty suggest one, or just call it “the dramatic one.”
>
> **Add the first plant**

### Error state

Exact copy:

> **The plant list did not load.**
>
> Planty will keep trying. Today's saved photos are still safe.
>
> **Try again**

### Gestures

- Swipe is limited to non-destructive “Take photo” and “Move to room” shortcuts.
- Delete is available only inside the plant menu and requires confirmation.
- Pull to refresh updates records without creating a new diagnosis.

## Notifications and deep links

Notifications are exception-only.
They deep-link to the relevant care card or diagnosis, never to a generic home screen.

Calm days produce no push notification by default.
An optional weekly reassurance summary may say:

> **Planty checked everyone.**
>
> Nothing needs you. Seriously, leave them alone.

A normal action notification may say:

> **Mona is ready for water.**
>
> Give her a slow drink when you're nearby. This is not urgent.

An urgent notification may say:

> **Move Mona away from the other plants.**
>
> Today's photo may show pests. Planty has the next step ready.
