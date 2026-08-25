# Planty UI concept

Planty answers one question: **what should I do while I am standing here with this plant?**

The emotional target is relief.
On a normal day the app confirms that current evidence was checked and that the user can safely do nothing.
When something needs attention, it reduces the decision to one physical action, a reason, and a low-friction way to record what happened.

## Product hierarchy

1. **Orient:** Today says whether the garden has a current all-clear or names the small number of actions due now.
2. **Capture:** Capture keeps the camera one tap away and protects the photograph before asking for metadata.
3. **Decide:** A photograph can become a timeline record, quick care action, identification, or contextual consultation.
4. **Remember:** Plant stories explain change over time with photographs, observations, notes, reminders, harvests, and only the sensor evidence that clarifies a finding.
5. **Manage:** More holds away mode, shelter state, owner questions, owner updates, harvest history, and settings so those workflows do not crowd Today.

## Information architecture

The app uses four native tabs:

- **Today** for current decisions and system freshness.
- **Capture** for camera and photo-library intake.
- **Plants** for the searchable library and plant stories.
- **More** for garden-wide workflows and configuration.

Diagnosis and consultation remain contextual to a plant or photograph rather than becoming a generic chat tab.
Settings opens as a sheet from the app shell and is also reachable from More.

## Visual system

Dracula Classic is the base palette, used semantically rather than decoratively.

| Role | Color | Use |
| --- | --- | --- |
| Canvas | `#282a36` | App background. |
| Raised surface | `#44475a` | Cards, controls, and photo frames. |
| Primary text | `#f8f8f2` | Headings and body copy. |
| Positive | `#50fa7b` | Current calm and completed actions. |
| Informational | `#8be9fd` | Sensor-backed context and links. |
| Attention | `#ffb86c` | An action is due without implying disaster. |
| Brand | `#bd93f9` | The lavender seal, ownership, and supporting identity. |
| Urgent | `#ff5555` | Confirmed danger or destructive action only. |
| Watch | `#f1fa8c` | A trend to observe without acting yet. |

Dracula comment blue is decoration only because it lacks enough contrast for small informative text on the canvas.
Typography uses system text styles, layouts grow vertically under Dynamic Type, and status is always stated in words rather than color alone.

The mascot is the lavender seal documented in [MASCOT.md](MASCOT.md).
It appears only in calm, success, and gentle guidance states, never beside urgent, destructive, stale, or error messages.

## Today

Today is designed from the calm state outward.
The common state is a satisfying confirmation with the number of plants checked and the time of the last complete run.

When action is needed, the hero contracts and the card leads with the action rather than a score or raw sensor reading.
Friend-owned plants carry a purple named ownership badge and sort first among equal-priority actions, but ownership is never represented as severity.

Completing the final action resolves the screen into calm.
Postponing preserves the verdict and moves it by an explicit interval; it never acknowledges work that was not done.

## Capture and consultation

The camera is the primary surface rather than a button inside a form.
Plant context may arrive from Today or a story, but the shutter remains useful without a selection because identification and plant creation can happen after the image is safe.

After capture, the user can save the photo, record a common care action, add a note, create a plant, or ask what looks wrong.
There is no required moisture field, species questionnaire, or journal entry.

Consultation separates observed evidence, likely interpretation, and the next physical action.
It can inspect historical photos and records or accept a current image, and it may write only through the constrained `planty agent` interface.
It must use uncertainty language and never promote a single photograph into a diagnosis it cannot support.

## Plant stories

A story is chronological and photo-led.
Narrative findings connect the images, observations and notes record what happened, and sensor evidence is disclosed only when it explains the conclusion.
Charts are absent from the default story because the app should make the decision instead of handing telemetry interpretation back to a beginner.

## Motion and feedback

- The calm mascot may make one subtle motion after a successful load and then stop.
- Completing an action collapses its card with light haptics.
- Saving a photograph confirms immediately and places it at the top of the story.
- Long-running model work names real stages rather than rotating fake progress phrases.
- Reduce Motion replaces movement with crossfades.

## Accessibility

- Interactive targets are at least 44 by 44 points.
- Labels and icons carry status and ownership in addition to color.
- Photo accessibility descriptions name the date, viewpoint, and visible finding rather than a filename.
- Dynamic Type wraps metadata before truncating it.
- The main shutter is at the bottom and has the VoiceOver label “Take plant photo.”
- The mascot is one decorative element with a concise description.
- Reduce Motion, Differentiate Without Color, and Increased Contrast are product requirements.

## Deliberately omitted

- Sensor dashboards.
- Care streaks and achievements.
- A global health percentage.
- Fixed watering calendars.
- Required journal entries.
- Community or encyclopedia surfaces.
- A general chat tab.
- Permanent alarm treatment for friend-owned plants.

These features optimize engagement or expose raw machinery without improving the decision Planty exists to make.
Unresolved implementation work belongs in [ROADMAP.md](../ROADMAP.md), not in a second backlog here.
