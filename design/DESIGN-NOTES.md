# Planty design notes

## The central decision

Planty should optimize for **confident inaction**, not engagement.
That is unusual enough to be the design's organizing principle.
Success means the user opens the app, understands that nothing needs doing, feels reassured, and leaves.

This choice has a cost: daily active time and conventional engagement metrics will look weak.
That is a product analytics problem, not a reason to manufacture chores.
Useful measures are avoided mistakes, successful captures, action completion, diagnosis follow-through, and whether the user still trusts the calm state after several weeks.

## A correction to the three-job framing

The stated jobs are Capture, Diagnose, and Glance, in that order.
I would keep all three but reject that order as the information architecture.

“Glance” sounds least important, yet it determines whether the user should act at all.
The app should open with orientation, make capture permanently adjacent, and treat diagnosis as a contextual continuation of a photo.
Making Diagnosis a tab would be a bad chatbot-shaped abstraction: it starts with an empty box and asks the user to supply context the system already has.

The resulting hierarchy is Today, Snap, Plants.
Diagnosis lives inside Snap and plant history.

## Decision: photo-first capture

### Considered alternatives

1. Pick event type, select a plant, then take a photo.
2. Select a plant, fill optional details, then take a photo.
3. Open directly to the camera, infer or confirm the plant afterward, then attach an event if useful.

### Outcome

Use camera-first capture.
The user's hands and attention are constrained, and the photo is the highest-value durable evidence.
Classification can wait until the image is safe.

The downside is potential plant misidentification.
The UI mitigates that by carrying context from the source care card, remembering recent plants, and requiring confirmation when confidence is low.

## Decision: native tab bar over a custom floating dock

A large floating pink camera control would look distinctive, but a custom navigation bar brings poor Dynamic Type behavior, uncertain VoiceOver ordering, and unnecessary competition with the system camera gesture area.
The prototype uses a standard three-item `TabView` and makes Snap the center item through position and pink tint.

If iOS 26's shipping tab APIs offer a first-class prominent action role with equivalent accessibility, the implementation can adopt it.
The design should not fake that role with a bespoke control before testing the native behavior.

## Decision: ownership is metadata, not severity

Friend-owned plants get a persistent purple named badge and sort first among equal-priority actions.
They do not receive permanent warnings or a separate anxious home screen.

The rejected alternative was dividing Today into “Plants you cannot kill” and “Your plants.”
That is funny once and emotionally corrosive every day.
The app can acknowledge the stakes without turning friendship into alarm chrome.

## Decision: stories over charts

Photo history is the product's differentiated evidence, so each plant gets a visual narrative with dated photos and short causal statements.
The default view does not include time-series moisture, humidity, or temperature plots.

Charts are attractive to an engineer and would be easy to overbuild.
They also shift interpretation back onto a beginner.
Relevant sensor windows remain available under “Why Planty thinks this” for trust and debugging, but they support the conclusion instead of becoming the interface.

## Decision: do not claim health

The calm copy says no action is needed based on current evidence.
It does not say a plant is healthy.
That distinction matters because the backend can miss visual problems between photos and sensors cannot observe many failure modes.

Similarly, the UI distinguishes an evaluated calm state from missing or stale evidence.
An analysis outage can never fall through to a reassuring empty screen.

## Decision: one recommendation, then evidence

Care cards lead with a single physical instruction and place the explanation beneath it.
Diagnosis responses separate observation, likely interpretation, and today's action.

The rejected alternative was a severity score with several possible remedies.
A score is pseudo-precision, and a menu of remedies leaves the beginner holding the same decision he asked Planty to make.

## Decision: humor has a blast radius

The starfish mascot and slightly dumb copy belong in calm, empty, and success states.
Normal action copy can be warm.
Urgent, destructive, stale, and error states use plain language.

This prevents the brand from becoming flippant when a borrowed plant may genuinely be in trouble.
“Nothing needs you. Seriously, leave them alone.” is funny and useful.
“Oops, root rot!” would be asshole UX.

## Rejected alternatives

### Gamified care streaks

Rejected because the correct behavior is often not touching the plant.
A streak would reward opening, watering, or logging regardless of need and could directly cause overwatering.

### A global health percentage

Rejected because it obscures uncertainty, invites compulsive checking, and does not tell the user what to do.
Plain states such as All good, Watch, Needs care, Urgent, and Unknown are more honest.

### A fixed watering calendar

Rejected because environmental evidence, not calendar cadence, should decide watering.
Calendar-like reminders are suitable only for follow-up photos or explicitly postponed actions.

### A general chatbot tab

Rejected because a blank composer has no plant, photo, or moment attached.
Conversation begins from evidence and stays attached to that plant's history.

### Dense per-plant dashboards

Rejected because the app is a field tool, not the source of truth or an operations console.
Raw observations and backend diagnostics can exist in a secondary evidence disclosure for debugging.

### Mascot as the shutter button

Rejected because a novel control makes the most important action less obvious and gives VoiceOver users a needlessly cute mystery target.
The shutter remains a familiar camera control.

### Mandatory before-and-after pairs

Rejected because they increase capture friction and will reduce the longitudinal dataset.
Planty can prompt for a second angle only when the first image is genuinely insufficient.

## Accessibility decisions

Dracula Classic is dark-first, but the original comment blue is too dim for informative small text against the background.
It remains available for decoration while a lighter secondary-text token handles metadata.

Color never carries status or ownership alone.
Cards use labels, SF Symbols, and VoiceOver descriptions.
The layout favors vertical stacks so accessibility text sizes do not crush several columns into unreadable fragments.

Plant photos require authored accessibility descriptions from the same analysis that creates the visual finding.
“Image, August 18” is not enough; a useful label is “Mona on August 18 from the front; two new leaves opening and lower yellowing unchanged.”

## Prototype scope

The prototype contains four essential experiences:

1. Today in calm and action states.
2. Snap before and after a mock capture.
3. The contextual diagnosis conversation.
4. Mona's visual plant story.

It uses generated SwiftUI surfaces and SF Symbols instead of bundled sample photography.
That keeps the prototype self-contained and avoids implying that decorative placeholder photos represent diagnostic quality.
The production design needs real-photo testing across dark foliage, pale walls, cabinet grow lights, and close leaf detail.

## Open questions

### Identity and plant selection

- Can the backend identify a plant reliably from image plus location and recent context, or must the app always request confirmation?
- Does the app know proximity to a room or cabinet through Home Assistant context, Bluetooth, or only recent selection?
- Can one sensor map to multiple pots, and how should Planty explain ambiguous sensor evidence?

### Verdict semantics

- What makes a daily verdict current enough for the calm state?
- How are analysis failure, missing sensor data, and no recent photo represented independently by the API?
- Does the backend return evidence citations that the UI can render without inventing causal language?
- Which care actions are safe for the model to recommend directly, and which require human escalation?

### Ownership and responsibility

- Is there one friend owner or could plants belong to several people?
- Does the friend need a handoff report when the sabbatical ends?
- Are any plants explicitly high-value, toxic to pets, or otherwise higher stakes than ownership alone conveys?

### Capture pipeline

- Does an observation upload immediately, queue offline, or require a backend-issued record first?
- Can diagnosis continue asynchronously and notify the user when ready?
- How many historical images can the vision pipeline compare, and does it align viewpoints?
- Should the app guide consistent framing with a translucent previous-photo overlay?

The previous-photo overlay is especially promising because comparison quality depends on consistent viewpoints.
It is absent from the first prototype because the brief does not establish whether the backend stores crop and camera geometry needed to make the overlay trustworthy.

### Action lifecycle

- Can recommendations expire or be superseded while the user has Today open?
- What is the difference between completed, postponed, dismissed, and contradicted actions in the backend?
- Should “watered by hand” require an amount, or is amount intentionally avoided because it is unreliable manual data?

My recommendation is to avoid volume entry unless a specific plant's care protocol genuinely depends on it.
“Slow drink until water reaches the tray” is more reproducible than asking a beginner to estimate milliliters while holding a can.

### Privacy and sharing

- Do plant photos risk capturing people, room interiors, documents, or location metadata?
- What photo retention and deletion behavior does the user expect?
- Can the friend see their plants or receive any automatic report?

No sharing should be inferred from ownership.
A “Maya's plant” badge is local responsibility metadata until the product explicitly establishes consent and access.
