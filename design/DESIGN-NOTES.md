# Planty design decisions

## Optimize for confident inaction

Planty should reduce avoidable intervention rather than maximize engagement.
Success is a user seeing a current all-clear, trusting it, and leaving the app.
Useful measures are avoided mistakes, successful captures, completed care, consultation follow-through, and continued trust in the calm state.

## Orient before capture

Today remains the default because the user must know whether action is needed before opening a camera.
Capture is permanently adjacent, Plants preserves history, and More keeps infrequent garden workflows out of the daily decision.
Diagnosis remains contextual to evidence instead of becoming a blank chatbot tab.

## Protect the photo first

Capture starts with the camera or Photos picker and asks for classification afterward.
The photograph is the most valuable durable evidence, and a failed network request must not discard it.
Plant context carried from Today or a story improves accuracy without making selection a prerequisite for the shutter.

## Use native navigation

Planty uses a native four-item `TabView` rather than a floating custom dock.
The camera does not justify sacrificing Dynamic Type behavior, VoiceOver order, familiar gestures, or platform navigation conventions.

## Ownership is metadata

Friend-owned plants receive a named purple badge and sort first among equal-priority actions.
They do not receive permanent warning chrome or a separate anxiety dashboard.
Severity is determined by evidence, not by who owns the pot.

## Stories over dashboards

Photo history is the differentiated evidence, so each plant gets a visual narrative with observations and short causal findings.
Sensor windows support a conclusion inside an evidence disclosure; they do not become a default telemetry dashboard that asks a beginner to interpret raw lines.

## Do not claim health

Calm means no action is recommended from current complete evidence.
It does not mean the plant is universally healthy.
Stale, missing, and partial checks remain distinct states because a failed analysis cannot inherit reassuring empty-screen copy.

## One recommendation, then evidence

Care cards lead with one physical instruction and place reasoning below it.
Consultations distinguish observation, likely interpretation, and today's action.
A numeric health score or menu of competing remedies would recreate the decision the user asked Planty to make.

## Humor has a blast radius

The lavender seal and lightly dumb copy belong in calm, empty, and success states.
Urgent, destructive, stale, and error states use plain language without the mascot.
The joke cannot be allowed to make a real risk feel unserious.

## Accessibility is structural

The app uses native controls, system text styles, vertical layouts, labels in addition to color, and useful photograph descriptions.
The current forced-dark appearance, incomplete Dynamic Type work, and unintentional iPad scaling are gaps, not design intent, and are tracked in [ROADMAP.md](../ROADMAP.md).

## Resolved questions

- A verdict is current enough for calm for 36 hours, subject to the service's explicit stale and run-completeness signals.
- Garden-wide judgment runs distinguish partial and interrupted analysis from a complete result.
- Verdicts carry evidence and the model that answered.
- Only one verdict per plant remains open, and care completion is idempotent.
- Several owners are supported, owner questions are durable, and editable seven-day owner updates ship through Messages.
- Capture uploads immediately and remains retryable on failure.
- Photograph comparison ships when two or more images exist.
- Unknown toxicity remains unknown rather than defaulting to safe.

## Still open

- Photo privacy, retention, deletion, and accidental room or location metadata capture.
- Whether asynchronous consultation completion should notify the user.
- Whether consistent-viewpoint overlays can be trustworthy with available camera metadata.
- Whether owners should ever receive direct access rather than a caretaker-authored message.
- Which authenticated network boundary would be acceptable if Planty ever leaves the LAN.

Implementation work belongs in [ROADMAP.md](../ROADMAP.md).
Decisions with real alternatives and consequences belong in [adr/](../adr/).
