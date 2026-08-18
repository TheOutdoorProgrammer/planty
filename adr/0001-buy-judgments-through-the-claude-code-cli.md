# Buy judgments through the Claude Code CLI

## Context and Problem Statement

Planty asks a model four questions: what to do about a plant today, what a photo timeline shows, what a plant probably is, and what killed one.
All four went through the Anthropic API on a metered key.

That key is a second bill on top of a Claude subscription that is already paid for and already entitles the same models.
A daily verdict per plant is small, roughly three cents a call at Opus with medium effort, but the vision calls are user-triggered from the phone and have no ceiling, and paying twice for the same capability is the part that grates rather than the absolute number.

The question is whether a service can spend a subscription instead of a metered key, and what that costs everywhere else.

## Considered Options

- **Keep the metered API key.** Nothing changes.
- **Shell out to `claude -p`.** The CLI authenticates against the subscription and returns the answer.
- **Point the SDK at a translating proxy.** Run something that speaks the Messages API and fulfils it with subscription credentials underneath.

## Decision Outcome

Chosen: **shell out to `claude -p`**, behind a `judge.Backend` interface with the API path kept as the second implementation.

The interface is the load-bearing part of this decision.
All four calls already had one shape (system prompt, a conversation of text and images, a JSON schema, an effort level), so they collapse into a single `Judge(ctx, Request) (string, error)`, and `PLANTY_JUDGE` picks who answers it.
Neither way of paying is written into the judgments themselves.

`--json-schema` turned out to be the thing that made this viable: the CLI validates structured output the same way `OutputConfig.Format` does, and hands back an already-checked object.
Without it this would have been prompt-and-hope parsing, and the answer would have been no.

### Consequences

Good:

- The subscription pays, and the metered key can be deleted from the cluster.
- The API backend still exists and is one environment variable away, so this is reversible without a rewrite.
- `--safe-mode` and `--tools ""` mean a judgment is a completion, not an agent: no ambient `CLAUDE.md`, no hooks, no MCP servers leaking into a plant verdict.

Bad, and accepted:

- **The image goes from about 15 MB to about 320 MB.** Distroless is gone, because the CLI needs a filesystem and somewhere to write. The native musl build keeps a Node runtime out of it, which is the only reason the number is 320 and not far worse.
- **Vision calls got slower.** Images cannot ride inline on a CLI prompt, so they are staged as files and read back with the `Read` tool, which is an extra agent turn. A measured identify against a 2.8 MB phone photo took 21 seconds, against roughly 4 through the API.
- **Judgments now compete with interactive work for the same rate limit.** A daily digest is nothing; a phone session hammering diagnosis is not.
- **`readOnlyRootFilesystem` needed two `emptyDir` mounts** to stay true, and the memory limit is now set by the CLI's heap rather than by the Go service.
- **Multi-turn diagnosis is flattened into a transcript in one prompt,** because the CLI takes a single string. The conversation survives; its structure does not.

The proxy option was rejected because it is a translation layer that fails quietly: it would have to reconstruct Messages API response shapes, and every gap between the real API and the imitation shows up as a subtly wrong verdict rather than an error.
A `Backend` interface inside Planty makes the same swap explicit, in a place where a wrong answer is a compile error instead of a runtime surprise.
