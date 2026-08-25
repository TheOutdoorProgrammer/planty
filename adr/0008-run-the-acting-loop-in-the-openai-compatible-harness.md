# Run the acting loop in the OpenAI-compatible harness

## Context and Problem Statement

[ADR 0007](0007-choose-a-model-per-job.md) introduced per-job model selection and an OpenAI-compatible chat-completions harness.
It deliberately refused OpenAI-compatible models for `consult` and `ask` because those jobs may run constrained Planty commands and fetch trusted reference pages, while the first harness could only produce one response.

That restriction made provider choice misleading.
A model could satisfy the vision and structured-output requirements but still be unavailable for the two interactive jobs because tool execution existed only inside the Claude Code CLI backend.
The allowed operations and their safety rules belong to Planty, not to a particular model vendor's CLI.

## Considered Options

- Keep acting jobs permanently on the Claude Code CLI.
- Let each provider supply its own agent runtime and permission model.
- Implement one bounded tool loop in Planty and expose the same constrained tools through the OpenAI-compatible protocol.

## Decision Outcome

Chosen: **implement one bounded tool loop in Planty and expose the same constrained tools through the OpenAI-compatible protocol**.

An acting request receives a request-scoped toolbox containing only `planty_agent` and, when a trusted-host allowlist exists, `web_fetch`.
`planty_agent` executes an argv directly rather than invoking a shell, requires the `planty agent` prefix, and still passes every command through the existing refusal gate.
`web_fetch` accepts HTTPS only, restricts the hostname to the request's trusted allowlist, caps the response size, and reduces the response to readable text.

The conversation is capped at twelve tool rounds.
A model that does not produce a final schema-constrained answer by then fails instead of looping indefinitely against a subscription or metered endpoint.
Tool definitions are built per request so a conversation cannot inherit authority from another job.

This decision supersedes only ADR 0007's conclusion that acting jobs must remain on Claude.
ADR 0007's per-job selection, provider abstraction, compatibility checks, and persisted assignments remain in force.

### Consequences

Good:

- `consult` and `ask` can run on any configured model whose capabilities have been verified for vision, schema, and tools.
- The authorization boundary remains in Planty instead of being delegated to provider-specific agent behavior.
- Provider replacement no longer changes which garden commands or reference sites a model may reach.
- Tool-loop behavior is unit tested and can be exercised against a live compatible provider.

Bad, and accepted:

- Planty now owns multi-round conversation orchestration, tool-call decoding, output clipping, and provider quirks.
- OpenAI-compatible endpoints differ in how faithfully they implement tool calls and strict JSON schemas, so the live capability catalogue must remain evidence-based.
- The built-in text extraction for trusted pages is intentionally simple and is not a general browser or HTML parser.
