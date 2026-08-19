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
- Photographs can be **offered rather than attached**, which the API cannot do at all. Staged as files with the `Read` tool available, the model opens one only when it would change the answer: asked when a plant was last watered it answers from the log in seven seconds and reports looking at nothing, and asked what colour the leaves are it opens the photo and takes twice as long. Through the API an image block sent is an image block read and paid for, so that backend names the photographs and says it has not seen them.

Bad, and accepted:

- **The image goes from about 15 MB to about 320 MB.** Distroless is gone, because the CLI needs a filesystem and somewhere to write. The native musl build keeps a Node runtime out of it, which is the only reason the number is 320 and not far worse.
- **Vision calls got slower.** Images cannot ride inline on a CLI prompt, so they are staged as files and read back with the `Read` tool, which is an extra agent turn. A measured identify against a 2.8 MB phone photo took 21 seconds, against roughly 4 through the API.
- **Judgments now compete with interactive work for the same rate limit.** A daily digest is nothing; a phone session hammering diagnosis is not.
- **`readOnlyRootFilesystem` needed two `emptyDir` mounts** to stay true, and the memory limit is now set by the CLI's heap rather than by the Go service.
- **Multi-turn diagnosis is flattened into a transcript in one prompt,** because the CLI takes a single string. The conversation survives; its structure does not.
- **Argument order is load-bearing.** `--tools` takes a list, so a prompt appended after it is read as one more tool name and the call dies asking for input it was handed. Every prompt goes after a `--`. This shipped once, breaking exactly the text-only calls, because every live test at the time happened to send a photograph and so ended on a different flag.

### Sessions

A conversation resumes rather than re-reads. The first turn passes `--session-id` with the conversation's own id, later turns pass `--resume` and send only the new question; the record and every earlier turn stay in the session.

Measured over four turns against a plant with sixty observations, billed input per turn went `3110, 3133, 3156, 3177` when replaying and `3104, 10, 10, 10` when resuming.
Replaying is slightly cheaper for a two-turn exchange, because establishing the session writes a large cache entry, and it breaks even at three.
From there resuming is roughly half the cost per turn and the gap widens, since replaying grows with the conversation while resuming does not.

Sessions live in the CLI's own state directory, which is an `emptyDir` and dies with the pod. A conversation can therefore outlive the session it was using, so a resume that finds nothing falls back to replaying the transcript: slower, still correct, and the only acceptable way for this to fail.
One-shot work (the daily verdict, identification, an autopsy) keeps `--no-session-persistence`, because there is nothing to continue and a session per verdict is a disk filling up for no reason.

The proxy option was rejected because it is a translation layer that fails quietly: it would have to reconstruct Messages API response shapes, and every gap between the real API and the imitation shows up as a subtly wrong verdict rather than an error.
A `Backend` interface inside Planty makes the same swap explicit, in a place where a wrong answer is a compile error instead of a runtime surprise.
