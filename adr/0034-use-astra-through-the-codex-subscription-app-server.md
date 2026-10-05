# 34. Use Astra through the Codex subscription app-server

Date: 2026-10-05

## Status

Accepted.

## Context and Problem Statement

The owner retired Claude and explicitly selected the Codex subscription for Astra instead of metered OpenAI API billing.
Planty needs structured answers, current and historical photographs, and its existing constrained acting tools.
This replaces the Claude purchasing and transport choice in [ADR 0001](0001-buy-judgments-through-the-claude-code-cli.md); its backend abstraction remains useful.

## Considered Options

1. Use the supported local Codex app-server over stdio with a dedicated subscription login.
2. Use the metered OpenAI Responses API.
3. Reuse Symphony's existing persistent Codex transport.
4. Connect to a remote Codex WebSocket app-server.

## Decision Outcome

Chosen: **option 1**.

Run pinned Codex 0.154.0 as a per-request stdio subprocess. Require ChatGPT authentication and the exact gpt-6-astra model, without API billing fallback. Keep Planty's database, Home Assistant credentials, personal settings and unrelated environment variables out of the subprocess.

Start each thread with explicit `environments: []`, removing native shell and filesystem tools. Register only dynamic tools from Planty's existing toolbox and enforce the request's Acting grants. Reuse its historical-photo selection and telemetry; send current images directly and preserve conversation roles. Require strict structured output.

Use a dedicated Planty login in a writable persistent Codex home. Hold an interprocess file lock for the entire subprocess lifetime, serializing refresh across the API and daily job. Seed production credentials once using supported Codex login, with restrictive permissions. A personal session clone is unsuitable for deployment because refresh in another process can invalidate it. On the existing single-node cluster, both workloads mount the same ReadWriteOnce volume. Do not reset it from a static Secret on startup.

The container ships Codex's checksum-verified native musl binary, replacing Claude. Synthetic live tests cover vision, strict schema, history, offered photos, permitted tool execution and denied native environment access. These prove capabilities, not deployed recovery.

Sources: [Codex authentication](https://developers.openai.com/codex/auth/), [CI authentication](https://developers.openai.com/codex/auth/ci-cd-auth), and the pinned [empty-environment tool specification test](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/core/src/tools/spec_plan_tests.rs#L1217).

## Consequences

### Good

- Uses the owner's chosen subscription and retains the existing model-job and tool safety contracts.
- Supports photographs and structured answers without granting a general shell or exposing application credentials.
- Durable serialized authentication survives pod replacement and prevents API/job refresh races.

### Bad

- Codex app-server protocol and model capabilities must be checked when upgrading the pinned binary.
- Subscription limits are shared with other account usage; serialized requests queue behind active judgments.
- Persistent OAuth state is a secret-bearing operational dependency that needs protected storage and occasional reauthentication.
- ReadWriteOnce sharing assumes the current single-node cluster; additional nodes require explicit placement or a storage redesign.

### Rejected because

- The Responses API introduces metered billing the owner explicitly rejected.
- Symphony's transport assumes persistent, broadly privileged text-oriented coding sessions; extracting it would preserve the wrong lifecycle and authority for Planty judgments.
- Remote WebSocket transport is experimental and would add a service and authentication boundary without solving Planty's constrained-tool requirement.
