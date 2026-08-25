# Fetch offered photos through the shared tool loop

## Context and Problem Statement

Planty offers a bounded set of historical photographs to a consultation instead of attaching every image to every question.
The Claude Code CLI could open one of those staged files, but the OpenAI-compatible harness received only their labels.
The model catalogue treated vision and ordinary tool use as sufficient, so a selected provider could see a newly attached image while being unable to inspect the timeline evidence the consultation promised.

The direct Anthropic API backend also has no shared acting loop.
Silently sending every historical photograph would restore parity by defeating the cost and privacy boundary that offered photos exist to provide.

## Considered Options

- Attach every historical photograph to every consultation request.
- Build provider-specific image selection behavior for each backend.
- Add one bounded historical-photo tool to Planty's shared acting loop and represent that access as a separate capability.
- Leave the direct Anthropic API eligible for acting jobs and disclose that its answers use less evidence.

## Decision Outcome

Chosen: **add a bounded historical-photo tool to the shared loop, model offered-photo access separately, and make the direct Anthropic API ineligible for acting jobs**.

The OpenAI-compatible harness exposes only the offered images already selected and bounded by the API.
The model asks for an image by its zero-based catalogue index, and Planty returns that one image as multimodal tool content.
The selected image is recorded in the answer trace without placing its encoded bytes in diagnostics.

Vision, structured output, ordinary tools, and offered-photo access remain distinct catalogue capabilities.
Assignment validation refuses a consultation model missing any required capability.
The iOS consultation screen also discloses when the configured default backend cannot inspect historical images.

The direct Anthropic API remains available for one-shot structured jobs such as daily assessment.
It is explicitly ineligible for `consult` and `ask` until it uses the same constrained acting and offered-photo loop.

### Consequences

Good:

- A provider sees no historical image unless it explicitly asks for that image.
- Provider selection now describes the evidence path the user actually receives.
- The acting authority and photo bounds remain owned by Planty rather than by provider-specific agent behavior.
- The direct API fails honestly instead of producing a plausible answer from less evidence than the UI promises.

Bad, and accepted:

- OpenAI-compatible endpoints must accept multimodal content in a tool result to qualify for acting jobs.
- A direct Anthropic API consultation is unavailable until that backend gains the shared loop.
- Capability entries require live verification when a provider changes its protocol behavior.
