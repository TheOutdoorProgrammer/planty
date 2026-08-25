# Planty screen behavior

This document defines the stable behavior and state semantics of the shipped app.
Exact wording may evolve with the implementation, but a copy edit may not weaken the truthfulness rules below.

## App shell

Planty launches into Today with four native tabs: Today, Capture, Plants, and More.
The selected tab uses the green product tint and a filled SF Symbol.
Tab labels remain visible at accessibility text sizes.

Settings opens as a sheet and contains API configuration, connection testing, data freshness, household notes, sensor links, build information, and per-job model assignments.
The current connection test proves API reachability only; APNs diagnostics remain roadmap work.

## Shared states

- **Loading** says what is happening and preserves useful previous content when possible.
- **Empty** explains whether setup is incomplete or the collection genuinely contains nothing.
- **Calm** requires a recent complete garden-wide judgment and names its freshness.
- **Watch** identifies a trend worth comparing later without prescribing action.
- **Needs care** leads with one physical action and then its evidence.
- **Urgent** uses explicit danger language, a label, and an icon rather than color alone.
- **Unknown** means current evidence is insufficient and is never equivalent to healthy.
- **Error** preserves retryable input and never masquerades as calm.

## Today

Today answers whether anything needs doing now.

The calm state says that nothing needs attention, how many plants were checked, and when the last complete run finished.
If the latest run is partial or unfinished, the screen says Planty needs a fresh look and keeps any still-open care actions from the last good run visible.
An API failure names the connection problem and retains the previous useful result when available.

Care cards show the plant, owner, location, action, short reasoning, and available completion or postponement controls.
Completing a care action records the matching observation and acknowledges the verdict in one idempotent operation.
Taking a photograph alone does not complete a care action.

Scheduled reminders appear as actions on Today and complete into their own observation and occurrence record.
Pull to refresh reloads state; it does not silently spend another model run.

## Capture

Capture supports the camera and Photos library.
The selected plant may be carried from another screen, chosen explicitly, inferred from a photo, or left unknown while the image is evaluated.

After capture the user can:

- Save the photograph to a selected plant.
- Record watered, repotted, fertilized, pruned, misted, moved, symptom, or note context where offered.
- Ask a contextual question with the image attached.
- Identify the subject and create a new plant while preserving the photograph as its first timeline entry.

A failed save leaves the photograph and entered context on screen for retry.
Discarding unsaved input requires confirmation.
On simulators or devices without an available camera, the Photos path remains fully usable.

## Plants

The library is searchable and visual.
Rows show the current photograph when available, name, location, ownership, and plain-language care state rather than sensor gauges.

A plant story combines:

- Current profile, ownership, toxicity, and care state.
- Photographs and their visual findings.
- Recent observations and readings.
- Open and historical verdicts with evidence.
- Notes, reminders, harvests, and consultations.
- Actions for capture, care recording, editing, and contextual questions.

Photograph comparison becomes available when at least two photos exist.
Unknown toxicity is labeled “Not checked” and is never styled as safe.

## Consultation

A consultation begins with a plant, a scratch photo, or a general question.
The response separates what the model observed, what it infers, what to do now, and which evidence it used.
When the model needs a historical photo, it may fetch only photographs and trusted reference sites offered to that request.

Acting consultations may run only the documented `planty agent` commands accepted by the server's refusal gate.
The interface shows recorded changes as results rather than trusting conversational prose as proof that a write happened.

## More

More contains garden-wide and infrequent workflows:

- Away-period creation, status, return briefing, and backup contact.
- Cold-risk plants and shelter or unshelter actions.
- Questions for plant owners and their recorded answers.
- Garden-wide harvest history and postmortem lessons.
- Owner-update generation and handoff to the native Messages composer.
- Settings and service configuration.

Owner updates remain editable and require the person to tap Send in Messages.
Planty never sends an iMessage or SMS silently.

## Notifications

Scheduled alerts arrive through APNs only.
The app registers its token with Planty, and the server fails a due job when no native delivery route works instead of switching to Home Assistant.

Tapping a notification currently opens the app normally.
Routing taps directly to a plant, reminder, cold action, or owner update is tracked in [ROADMAP.md](../ROADMAP.md).

## Destructive and irreversible actions

Archival, death recording, and discarding unsaved captures require explicit confirmation.
The service preserves historical records rather than deleting a plant as the normal lifecycle action.
Restoring archived plants in the iOS app remains roadmap work.
