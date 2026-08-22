# Per-job model selection + an OpenAI-compatible harness

Decision record: `adr/0007-choose-a-model-per-job.md`.
Comment inline on anything here before I build it.

## What this delivers

1. A third judge backend speaking OpenAI `/chat/completions`, pointable at any base URL (OpenCode Go, Zen, OpenAI, LiteLLM, Ollama).
2. A model chosen per job, persisted server-side, changeable from the phone.
3. A capability-annotated model catalogue so the picker cannot offer a blind model to a job that shows it photographs.
4. A fix for `Evidence.ModelVersion`, which currently records a constant rather than the model that answered.

## The six jobs

| Job | File | Photos | Tools | Effort | Volume |
| --- | --- | --- | --- | --- | --- |
| `Assess` | `internal/judge/judge.go:157` | no | no | medium | 13/day |
| `Identify` | `internal/judge/identify.go:76` | **yes, inline** | no | unset | on demand |
| `Consult` | `internal/judge/consult.go:159` | **yes, offered** | **yes** | medium | interactive |
| `Ask` | `internal/judge/scratch.go:77` | **yes, offered** | **yes** | medium | interactive |
| `Postmortem` | `internal/judge/postmortem.go:67` | no | no | high | on death |
| `OwnerUpdate` | `internal/judge/owner_update.go:40` | no | no | medium | interactive |

### Constraint worth arguing about

`Consult` and `Ask` grant the model `Bash(planty agent *)` plus fetching from an allowlist.
That agentic loop lives entirely in the Claude Code CLI backend; the harness answers in one shot and runs nothing.
So **those two jobs cannot move to an OpenAI provider without also writing a tool loop**, which is out of scope here.

I propose the picker **refuses** OpenAI providers for `Consult` and `Ask` rather than silently dropping their tools.
Losing the ability to say "water it" mid-conversation is not a degradation somebody should discover by accident.
Four of six jobs move; two stay on Claude until a tool loop exists.
Say the word if you want the tool loop in this change instead, and it becomes a much bigger one.

## Server

### 1. Model onto the Request

`internal/judge/backend.go`: add `Model string` to `Request`, beside `Effort` and `Acting`, which already vary per call site.
Empty keeps the backend's configured default.
Add `Model string` to `Outcome` so a backend reports what actually answered.

Deliberately **not** following the `Able()` precedent (`judge.go:30`), which mutates in place and returns the same pointer.
Concurrent HTTP handlers share one `Judge`; a `WithModel()` in that shape is a data race.

`apiBackend` (`api.go:41`) and `cliBackend` (`cli.go:127`) take `req.Model` when set, else `b.model`.

### 2. Providers

New `internal/judge/provider.go`: a provider is kind + base URL + credential + default model.

- `anthropic` and `claude-cli`: unchanged behaviour.
- `openai`: instantiable N times.

Configured by env (`PLANTY_PROVIDERS` as JSON, or repeated `PLANTY_PROVIDER_<NAME>_*`).
**Open question for you: which shape do you prefer?** I lean on a single JSON blob so a provider is one secret, not four.

### 3. The harness

New `internal/judge/openai.go`, `openaiBackend`:

| Request field | Wire |
| --- | --- |
| `System` | `messages[0]` role `system` |
| `Turns` text | `messages[]` content |
| `Turns` images | `image_url` with `data:<media>;base64,…` |
| `Offered` | named, not attached, matching `api.go:64` `withOffers` |
| `Schema` | `response_format: {type: "json_schema", json_schema: {name, strict: true, schema}}` |
| `Effort` | `reasoning_effort` |
| `MaxTokens` | `max_completion_tokens` |
| `Acting` | **unsupported**, returns an error rather than silently dropping tools |

Written against `net/http` + `encoding/json`, not an SDK.
The surface used here is six fields of one endpoint, an SDK is a dependency and a version treadmill for no gain, and hand-rolling keeps "any OpenAI-compatible endpoint" honest instead of inheriting a vendor client's assumptions.

`Offered` keeps `api.go`'s naming-without-attaching behaviour deliberately: attaching a month of photos to every question is the cost decision that comment already argues for, and the harness should not quietly reverse it.

### 4. Settings persistence

`internal/store/migrations/00017_model_assignments.sql`:

```sql
CREATE TABLE model_assignments (
    job        TEXT PRIMARY KEY,
    provider   TEXT NOT NULL,
    model      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Empty table means environment defaults still apply, so the deployment keeps working unconfigured and this is purely additive.
`internal/store/model_assignments.go` follows `push_devices.go`: `UpsertModelAssignment`, `ModelAssignments`, `DeleteModelAssignment`, validate then `ON CONFLICT DO UPDATE`.

### 5. Routes

Edit `api/openapi.json`, then regenerate both sides with `cmd/contractgen`.

- `GET /v1/models` : the catalogue, each entry carrying provider, id, and vision / schema / effort / tools flags.
- `GET /v1/model-assignments` : current job to model mapping, with which are defaults.
- `PUT /v1/model-assignments/{job}` : set one.
- `DELETE /v1/model-assignments/{job}` : revert to default.

Catalogue is each configured provider's `/v1/models` (so an unknown endpoint still works) annotated from an embedded capability table for models we know.
Unknown models stay selectable but are marked capabilities-unknown rather than hidden.

`job` becomes a generated wire enum in `api/openapi.json`, so it lands in `contract_generated.go` and `APIContract.generated.swift` together.

### 6. Wiring

`judge.New()` is called in three places (`cmd/planty/main.go:151`, `:205`, `:280`), each its own singleton.
Each gains a resolver reading `model_assignments` with an env fallback, and each call site passes its `Job`.

`internal/job/daily.go:112` stamps `outcome.Model` instead of `judge.Model`.

**Pre-existing coupling I am not fixing here, flagging so it is a choice:** `s.judge` is only set by `WithPhotos` (`internal/api/server.go:45`), so with no S3 configured, `/v1/ask`, `/v1/identify` and `/v1/owner-update` report unavailable even though none of them need object storage.
The settings routes will read the store directly and will not inherit that gate.

### 7. Tests

No fake `Backend` exists in the repo today.
Adding `stubBackend` is new but is exactly what per-job selection needs asserting against: that each call site sends its own job's model.
Harness tests follow `cli_test.go`, constructing the backend directly and asserting the outbound JSON against an `httptest` server.
Store tests via `internal/pgtest`, API tests via `internal/api/api_test.go`'s `newServer(t)`.

## iOS

### Reuse, not new UI

`ios/Planty/Screens/ManagedChoicePicker.swift` already solves this exact problem.
Its own doc comment states the rule: closed protocol concepts stay `Picker`s backed by enums, open server-owned vocabularies use `ManagedChoiceField`.
Server-supplied model IDs are the second kind, so the model picker is `ManagedChoiceField`, not a new control.

- `ios/Planty/Networking/ModelSettingsClient.swift`: three methods, `Patience.ordinary` (these do not wait on an LLM).
- `PlantyAPI.swift`: protocol entries plus 503 default implementations, so existing test doubles keep compiling.
- `ios/Planty/Models/ModelAssignment.swift`: `Codable, Sendable, Hashable, Identifiable`, explicit `CodingKeys`.
- `ios/Planty/Models/Enums.swift`: handwritten `label` for the generated `AIJob` enum, matching the file's stated cases-generated-labels-handwritten rule.
- `ios/Planty/State/ModelSettingsStore.swift`: `@Observable @MainActor final class`, `replace(api:isConfigured:)` + generation-guarded `load()`, owned by `AppSession` and reset in `updateConfiguration`.
- `SettingsScreen.swift`: a sixth section, `modelsSection`, between `freshnessSection` and `sensorsSection`. Six rows, one per job, each a `ManagedChoiceField` filtered to models that can do that job, with a footer sentence naming the consequence, matching the existing footers.

New `.swift` files are picked up automatically (`PBXFileSystemSynchronizedRootGroup`), so no `pbxproj` edit.
Tests in `ios/PlantyTests` with swift-testing and `IsolatedStubTransport`, following `ManagedChoicesTests.swift`.

## Commit sequence

Each commit stands alone as a PR.

1. `feat(judge): carry the model and report it on the outcome` : `Request.Model`, `Outcome.Model`, both existing backends, `daily.go` provenance fix.
2. `feat(judge): answer through any OpenAI-compatible endpoint` : provider config + `openaiBackend` + tests.
3. `feat(store): persist a model per job` : migration + store + tests.
4. `feat(api): expose the model catalogue and assignments` : openapi.json + regen + handlers + tests.
5. `feat(judge): resolve the model per job` : resolver, call sites pass their job.
6. `feat(ios): choose a model per job in settings` : client, model, store, settings section, tests.
7. `docs: record how models are chosen` : README + `deploy/README.md` + configmap.

## Open questions

1. **Provider config shape**: one `PLANTY_PROVIDERS` JSON blob, or per-provider env vars? I lean JSON.
2. **`Consult` / `Ask`**: refuse OpenAI providers (my proposal), or build the tool loop in this change?
3. **Default assignments** shipped in the configmap: I propose `Identify` stays Claude, `Assess` / `Postmortem` / `OwnerUpdate` move to OpenCode Go. Confirm.
4. **I cannot live-test.** There is no `OPENCODE_API_KEY` in the environment and your keys live in the cluster secret. I need either the key somewhere readable or permission to test from inside the cluster. Specifically unresolved: whether `qwen3.8-max` answers on `/chat/completions` (models.dev says yes) or `/messages` (the Zen docs say Qwen routes there). That decides whether your identification pick is reachable through the harness at all.
