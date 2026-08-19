# Let the conversation write, through exactly one command

## Context and Problem Statement

Asking about a plant was read-only.
The two most natural things to say to a plant app are "I watered it this morning" and "remind me to mist this twice a day", and it could do neither: it answered, and the person then had to go and record it themselves.

Giving the model the ability to act means giving it a tool, and the process it runs in holds a long-lived Home Assistant token that can operate a physical pump, a Postgres DSN, MinIO credentials, and a Claude OAuth token.
Its prompt is built partly from text a person typed: plant names, photo captions, care notes, the question itself.
So the question is not "can it act" but "what exactly can it do, and what stops it doing anything else".

## Considered Options

- **`Bash` restricted to `planty*`.** The obvious reading of "let it use the CLI".
- **A separate `planty agent` verb set, allowlisted.** The same idea with a much smaller surface.
- **An MCP server.** Typed tools, no shell at all.

## Decision Outcome

Chosen: **a `planty agent` verb set, granted through an allowlist and a hook**, and only during a consultation.

`Bash(planty *)` was rejected on inspection: the planty CLI can run the pump (`water`), migrate the schema, seed fixtures, fire notifications at a phone, and start another judgment (`autopsy`) that spends more tokens recursively.
Prefix-matching the binary would hand over all of it.
`planty agent` has three verbs (`log`, `remind`, `forget`), and there is deliberately no way to water anything from it.

MCP remains the better shape if the surface ever grows, because typed tools beat a command line the model has to construct. It was not needed for three verbs.

### How it is held shut

Two independent layers, because the first is a string match on a command line and the second is a process that has to agree with it.

1. `--permission-mode dontAsk` with `--allowedTools 'Bash(planty agent *)'`.
   `dontAsk` is what makes an allowlist a boundary: print mode starts in `manual`, where an allow rule only spares a prompt that could never have been shown, so everything else runs.
   Compound commands are split on `&&`, `;`, `|` and friends and every part must match independently, and command substitution defeats prefix matching outright.
2. A `PreToolUse` hook, which is `planty gate`: the same binary, so no interpreter or `jq` is added to the image.
   Hooks run *before* permissions are evaluated, so this also closes the gap left by the built-in read-only commands that no rule can reach.

Verified live against a fake `planty` that logs what it was asked to do: the model recorded a watering, refused `touch`, and refused a chained command in its own words. No probe file was ever created.

### Consequences

Good:

- "I watered it this morning" and "remind me to mist this twice a day" now work, which is most of what anyone says to a plant app.
- The daily verdict, identification and autopsies stay read-only. A scheduled judgment that recorded observations would be judging evidence it had written itself.
- `PLANTY_JUDGE_CAN_ACT=false` turns it off without touching anything else.
- Observations written this way are recorded with source `agent`, so the record always says who claimed a thing happened.

Bad, and accepted:

- The model constructs a command line, so it can get the syntax wrong. Validation catches it and the error goes back as text, but a typed tool surface would not have the failure mode at all.
- The allowlist is a string match. It is why there is a second layer.
- `--safe-mode` had to go. It does **not** exclude ambient permission rules, and it silently disables hooks, which would have removed layer 2 without saying so. Isolation is now `--setting-sources ''` and `--strict-mcp-config`, which also stopped ambient MCP servers attaching to judgments, something `--safe-mode` had been claiming to prevent and did not.
