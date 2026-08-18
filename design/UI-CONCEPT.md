# Planty UI concept

## The promise

Planty answers one question: **what should I do while I am standing here with this plant?**

The product should feel like a competent, cheerful friend who glances at the plant, tells you whether to act, and then gets out of the way.
It is not a remote control for the backend, a sensor dashboard, a care streak, or a gardening course.

The emotional target is relief.
On a normal day the app should confirm that the system is watching and that the user can safely do nothing.
When something does need attention, Planty should reduce the decision to one concrete action with a reason and an easy way to capture what happened.

## Product hierarchy

The brief's three jobs are useful, but their priority is not quite the same as the navigation hierarchy.
The user must orient before capturing: “Is there anything I need to do?” comes before “open a camera,” even if capture is the most important interaction once he is beside a plant.
Diagnosis is not a destination at all.
It is the conversational result of a capture.

The app therefore uses this task sequence:

1. **Orient:** Today says either “You're done” or names the small number of plants needing attention.
2. **Capture:** Snap is always one tap away and optimized for one thumb.
3. **Decide:** A photo can become a quick record or a diagnosis conversation.
4. **Remember:** Each plant's story explains change over time with photographs and short narrative findings.

## Information architecture

There are three persistent tabs:

- **Today** is the default and contains only current decisions.
- **Snap** is the center tab and the primary capture action.
- **Plants** is the library and the route into each plant's story.

Diagnosis is launched from a captured photo with “Something looks off.”
It is not a fourth tab because users do not arrive wanting a generic diagnosis form; they arrive with a particular plant and a current photo.

Settings, sensor status, account management, and backend details belong behind the profile menu.
They should not compete with the daily task.

## Visual system

Dracula Classic is the base, but it is used as a semantic system rather than rainbow decoration.

| Role | Color | Use |
| --- | --- | --- |
| Canvas | `#282a36` | App background |
| Raised surface | `#44475a` | Cards, photo frames, controls |
| Primary text | `#f8f8f2` | Headings and body copy |
| Quiet decoration | `#6272a4` | Dividers and non-text ornament only |
| Positive | `#50fa7b` | Calm confirmation and completed actions |
| Informational | `#8be9fd` | Sensor-backed context and links |
| Attention | `#ffb86c` | Action is due, without implying disaster |
| Mascot and capture | `#ff79c6` | Brand and primary capture affordance |
| Friend-owned | `#bd93f9` | Custody badge and subtle card edge |
| Urgent | `#ff5555` | Confirmed danger or destructive action only |
| Watch | `#f1fa8c` | Something to observe, not act on |

Dracula's comment blue does not have enough contrast for small body copy on the background.
It is deliberately excluded from informative text.
Secondary copy uses foreground at reduced hierarchy through weight and placement, or a lighter blue-gray token that passes contrast.

Typography uses system text styles with the rounded system design for display headings.
No text has a fixed point size.
Cards grow vertically under Dynamic Type instead of compressing into a horizontal dashboard row.

The pink starfish mascot appears only in calm, success, and gentle guidance states.
It never appears beside a severe alert because a grinning starfish next to “possible root rot” would make the product feel unserious in the wrong way.

## Ownership without anxiety

Friend-owned plants get a persistent purple `person.2.fill` badge containing the owner's name, such as “Maya's plant.”
Personal plants get a quieter “Mine” badge only where ownership could be ambiguous.
Friend plants sort first when action is required, but they do not get red chrome, warning triangles, countdown timers, or guilt copy.

The distinction communicates responsibility without pretending every borrowed plant is already an emergency.
Red is reserved for evidence of active harm.

## Screen 1: Today

Today is designed from the calm state outward.
The most common screen is a satisfying completion state, not an empty list.

```text
+--------------------------------------+
| Planty                         (face) |
|                                      |
|            \  *  /                   |
|          -- pink --   [tiny pot]     |
|            / | \                     |
|                                      |
|              You're done.            |
|     Nothing needs you right now.     |
|                                      |
|   8 plants checked  •  updated 8:04  |
|                                      |
|  +--------------------------------+  |
|  | All quiet in the greenhouse    |  |
|  | Next automatic check: tomorrow |  |
|  +--------------------------------+  |
|                                      |
|       [ Take a photo anyway ]        |
|                                      |
|  Today          [ Snap ]      Plants |
+--------------------------------------+
```

When action is needed, the calm hero contracts and action cards replace it.
The card leads with the action, not a score or raw moisture value.

```text
+--------------------------------------+
| Today                                |
|                                      |
| One thing. You've got this.          |
|                                      |
|  +--------------------------------+  |
|  | MAYA'S PLANT      Needs water  |  |
|  | Mona • Swiss cheese plant      |  |
|  |                                |  |
|  | Give it a slow drink. Stop     |  |
|  | when water reaches the tray.   |  |
|  |                                |  |
|  | Why: soil has stayed dry...    |  |
|  | [I'm here]       [Not now]     |  |
|  +--------------------------------+  |
|                                      |
| Everything else is okay.             |
|                                      |
|  Today          [ Snap ]      Plants |
+--------------------------------------+
```

“I'm here” opens capture already associated with Mona and offers a single-tap “Watered” record after the photo.
“Not now” postpones the card for a short, explicit interval; it never silently marks the task complete.

## Screen 2: Snap and quick record

The camera is the screen, not a small button in a form.
Plant selection sits at the top as a large chip, prefilled from the action card or the most recently viewed plant.
If Planty is not confident about the plant, it asks after the photo rather than blocking the shutter.

```text
+--------------------------------------+
| Cancel      Mona • Maya's      flash |
|                                      |
|  +--------------------------------+  |
|  |                                |  |
|  |       live camera preview      |  |
|  |                                |  |
|  |     Fit the whole plant in     |  |
|  |       the loose outline        |  |
|  |                                |  |
|  +--------------------------------+  |
|                                      |
|       [ gallery ]  ( shutter )       |
|                                      |
|         One photo is enough.         |
+--------------------------------------+
```

After capture, a bottom sheet offers three large exception-oriented actions:

```text
+--------------------------------------+
|               photo                  |
|                                      |
|  What happened here?                 |
|                                      |
|  [ Watered by hand ]                 |
|  [ Repotted ]                        |
|  [ Something looks off ]             |
|                                      |
|  Add a short note (optional)          |
|                                      |
|        [ Save photo only ]            |
+--------------------------------------+
```

There is no moisture field, care checklist, species questionnaire, or required note.
Routine observations already belong to the sensors and backend.

## Screen 3: Diagnosis conversation

Diagnosis begins with the photo and one plain-language prompt.
The first response must distinguish observed evidence from inference and give the next physical action.

```text
+--------------------------------------+
| < Mona             Looking closer…  |
|                                      |
|                       [your photo]   |
|       Something looks off.           |
|                                      |
|  +--------------------------------+  |
|  | The lower yellowing has spread |  |
|  | since July 20.                 |  |
|  |                                |  |
|  | Most likely: too much water,   |  |
|  | not too little light.          |  |
|  |                                |  |
|  | Today: don't water it. Empty   |  |
|  | any water sitting in the tray. |  |
|  |                                |  |
|  | I'll compare another photo in  |  |
|  | 3 days.                        |  |
|  +--------------------------------+  |
|                                      |
| [Ask a follow-up…]        [camera]   |
+--------------------------------------+
```

Suggested follow-ups are conversational: “Show me what changed,” “Could this be pests?”, and “What would make this urgent?”
There is no symptom taxonomy.
The response should never fabricate certainty from a single image; confidence belongs in language such as “most likely” and in a short “based on” disclosure.

## Screen 4: Plant story

History reads top to bottom as a story.
Large photos anchor time, and narrative findings bridge the photos.
Sensor evidence appears only when it explains a change.

```text
+--------------------------------------+
| < Plants                       •••   |
|                                      |
| Mona                                 |
| Swiss cheese plant   [Maya's plant] |
| [latest wide photo]                  |
|                                      |
| Doing okay                           |
| New growth is steady. Lower leaves   |
| are recovering after watering less. |
|                                      |
| AUG 18                               |
|  [photo]  Two new leaves are opening |
|           No action needed.          |
|       |                              |
| AUG 11                               |
|  [photo]  Yellowing stopped spreading|
|           after the Aug 8 change.    |
|       |                              |
| JUL 20                               |
|  [photo]  First lower-leaf yellowing.|
|                                      |
|        [ Take today's photo ]         |
+--------------------------------------+
```

Charts are deliberately absent from the default story.
“Evidence” can disclose the relevant sensor window for a finding, but a beginner should not have to interpret a moisture graph to decide whether to water.

## Plants library

The library is a searchable visual list grouped into “Maya's plants” and “Mine.”
Each row shows a recent photo, name, room, current plain-language status, and last-photo age.
It does not show sensor gauges.

The empty state is an onboarding path: “No plants yet. Add one with a photo; Planty can help with the name.”
Adding a plant is intentionally photo-first because species identification can happen after capture and be corrected later.

## Motion and feedback

- The calm mascot makes one slow, subtle wobble when Today finishes loading, then stops.
- Completing an action collapses its card and changes the summary to “That's it.” with light haptics.
- Saving a photo uses a short checkmark transition and immediately places the new entry at the top of the plant story.
- Diagnosis uses a normal conversational progress row: “Comparing with 6 earlier photos…” rather than an indefinite spinner with no explanation.
- Reduce Motion removes mascot movement and uses crossfades instead of card travel.

## Accessibility

- Every interactive target is at least 44 by 44 points.
- Status is always conveyed by words and icons in addition to color.
- The ownership badge reads “Friend's plant, belongs to Maya” in VoiceOver.
- Photo cards have descriptions containing date, viewpoint, and visible finding rather than filenames.
- Dynamic Type uses system styles through accessibility sizes; horizontal metadata wraps before it truncates.
- The main capture control is reachable at the bottom and has the VoiceOver label “Take plant photo.”
- Decorative mascot pieces are combined into one element and labeled “Planty, a pink starfish holding a plant.”
- The app respects Reduce Motion, Differentiate Without Color, and Increased Contrast.

## Deliberately omitted

- **No sensor dashboard.** The backend consumes telemetry; asking the user to interpret it defeats the product.
- **No care streaks.** They reward interacting with the app, not leaving a healthy plant alone.
- **No global health score.** A score creates anxiety without specifying an action and hides uncertainty behind fake precision.
- **No watering schedule calendar.** Watering on a fixed schedule is exactly how beginners overwater plants; actions should come from current evidence.
- **No required journal entry.** Photos and one-tap exception tags are the journal.
- **No community, encyclopedia, or achievements.** The user explicitly does not want another hobby.
- **No red badge count for friend plants.** Ownership is visible, but borrowed plants should not become a permanent notification threat.
- **No chat tab.** Conversation is contextual to a plant and a photo, not an empty general-purpose chatbot.

## The brief's risky ideas

The mascot is useful, but making it ever-present would cheapen diagnoses and consume space needed for a one-handed camera.
It is a tone tool, not a navigation control.

Daily LLM verdicts are also a product risk if the app presents them as fresh certainty.
The UI must show when evidence was last updated, distinguish “no concern found” from “no recent data,” and never turn a failed daily analysis into the reassuring calm state.

Finally, “usually nothing” cannot mean hiding system freshness.
The calm state earns trust only if it quietly says how many plants were checked and when.
