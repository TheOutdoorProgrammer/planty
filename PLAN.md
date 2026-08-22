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

### The tool loop

`Consult` and `Ask` grant the model `Bash(planty agent *)` plus fetching from an allowlist.
That agentic loop lives entirely in the Claude Code CLI backend, which runs the tools itself.
The harness has to run them, so the harness gets a function-calling loop.

Two functions are offered, and only when `Acting` is set:

- **`planty_agent(command)`** runs `<binary> agent <verb> …`.
  Every command passes through the same refusal logic the Claude Code hook uses.
  `internal/agent/gate.go` already implements it as an unexported `refuse(command) string`; it gets exported as `Refuse` and `Gate` keeps calling it, so both paths share one boundary rather than growing a second copy that drifts.
- **`web_fetch(url)`** fetches a page and returns its text, refusing any host outside `agent.Trusted`.

There is deliberately no `web_search`.
The service has no fetcher or search provider of its own today (Claude Code supplied both), and every trusted site exposes its own search as a URL on that same host, so searching is `web_fetch` against a search page.
That keeps the allowlist as the single boundary and avoids introducing a search API key and a new secret for five known sites.

The loop caps iterations and total tool calls, and turns each round into `judge.Step` entries (`StepThought` for reasoning, `StepAction` for a call and its output) so the phone keeps showing what the model actually did.
That matters because `Answer.Steps` is already rendered by `StepsDisclosure.swift`; a job that moved provider and silently stopped explaining itself would be a regression.

### Capability gating is enforced, not advertised

A model that cannot read an image is **unassignable** to a job that shows it one.
This is enforced in the store and the handler, not just filtered in the picker: the API returns 422 for an incapable pairing, and the resolver refuses to start with one.
A picker that merely hides bad options still lets a stale client, a direct `PUT`, or a hand-edited row put `Assess` on a blind model.

Required capabilities per job, derived from the call sites:

| Job | Needs vision | Needs schema | Needs tools |
| --- | --- | --- | --- |
| `Assess` | no | yes | no |
| `Identify` | **yes** | yes | no |
| `Consult` | **yes** | yes | **yes** |
| `Ask` | **yes** | yes | **yes** |
| `Postmortem` | no | yes | no |
| `OwnerUpdate` | no | yes | no |

Every job needs schema, because every call site unmarshals a validated result.
On the verified Go roster that alone rules out `deepseek-v4-flash-vision-exp`, and vision rules out `gpt-5.6-luna` for three jobs.

### Claude is a choice in the picker, not the fallback underneath it

The catalogue lists Claude models alongside every other provider, so a job can be moved onto or back off Claude from the phone like any other option.
**Choosing a Claude model runs `claude -p`**, the existing `cliBackend`, which spends the subscription rather than a metered key. That is what ADR 0001 decided and nothing here reverses it.

Capability is the intersection of what the model can do and what its **backend kind** can do:

| Provider kind | Vision | Schema | Tools | Effort |
| --- | --- | --- | --- | --- |
| `claude-cli` (subscription, `claude -p`) | yes | yes | **yes** | yes |
| `openai` (any compatible endpoint) | per model | per model | **yes**, via the loop | per model |

The metered `anthropic` backend stays in the code and stays selectable by environment (`PLANTY_JUDGE=api`) for anyone without a subscription, but it is **not offered in the picker**.
It has never implemented `Acting` and would be silently tool-less for two jobs, and putting a second way to buy the same Claude models in front of a person is a bill waiting to be run up by accident.

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
| `Acting` | `tools` function definitions plus the loop described above |

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
2. `feat(judge): describe what each job needs and each model can do` : `Job`, capabilities, catalogue, provider config.
3. `feat(judge): answer through any OpenAI-compatible endpoint` : `openaiBackend` one-shot + tests.
4. `feat(judge): let the harness run planty and read trusted pages` : exported `agent.Refuse`, the tool loop, `web_fetch` + tests.
5. `feat(store): persist a model per job` : migration + store + capability validation + tests.
6. `feat(api): expose the model catalogue and assignments` : openapi.json + regen + handlers + tests.
7. `feat(judge): resolve the model per job` : resolver, call sites pass their job.
8. `feat(ios): choose a model per job in settings` : client, model, store, settings section, tests.
9. `docs: record how models are chosen` : README + `deploy/README.md` + configmap.

## Verified against the live endpoint

Probed `https://opencode.ai/zen/go/v1/chat/completions` directly on 2026-08-22.
`GET /v1/models` returns 29 ids, so the catalogue endpoint can be built dynamically as planned.

| Model | text | `response_format` | `reasoning_effort` | image via data URI |
| --- | --- | --- | --- | --- |
| `qwen3.8-max` | 200 | 200 | 200 | **200**, answered correctly |
| `kimi-k3` | not tested | 200 | not tested | **200** |
| `mimo-v2.5` | not tested | 200 | not tested | **200** |
| `gpt-5.6-luna` | 200 | 200 | 200 | **400** |
| `deepseek-v4-flash` | 200 | 200 | 200 | 400, text-only |
| `deepseek-v4-pro` | not tested | not tested | not tested | 400, no image support |
| `deepseek-v4-flash-vision-exp` | not tested | **400, no `response_format`** | not tested | 400 |
| `muse-spark-1.2-contributor` | not tested | not tested | not tested | 400 |
| `grok-4.5` | **503** | **503** | **503** | **503** |

Findings that change the plan:

- **`qwen3.8-max` answers on `/chat/completions`.** models.dev was right and the Zen docs' Qwen-routes-to-`/messages` note does not apply to the Go endpoint. The identification pick is reachable through this harness.
- **`grok-4.5` is unavailable**, text and vision alike: "Endpoint is unavailable" on every call despite being listed in `/v1/models`. Drop it from consideration.
- **`gpt-5.6-luna` cannot accept images here.** It is a Responses-API model and the chat/completions shim returns 400 with an empty message body for multimodal content. It stays the text workhorse and must never be offered for a job that shows photographs.
- **The registry is wrong in both directions.** `deepseek-v4-flash-vision-exp` advertises `structured_output: true` and rejects `response_format` outright; `mimo-v2.5` leaves it unset and handles it fine. A static capability table copied from models.dev would misinform the picker, so the catalogue must record what was actually observed and be correctable.
- `muse-spark-1.2-contributor` fails vision, which retires the model-trains-on-your-data tradeoff without needing a decision.
- Responses carry `"cost": "0"`, confirming subscription metering rather than per-request billing.
- `gpt-5.6-luna` returns `"finish_reason": null` on success. The harness must not depend on `finish_reason`.

**The verified vision + schema set on OpenCode Go is exactly three: `qwen3.8-max`, `kimi-k3`, `mimo-v2.5`.**
`qwen3.8-max` is also the best benchmarked of the three for identification, so it stands as the pick on evidence rather than inference.

## Decisions taken

1. **Provider config** is one `PLANTY_PROVIDERS` JSON blob, so a provider is one secret rather than four correlated variables.
2. **The tool loop is in scope**, so all six jobs can move to an OpenAI provider.
3. **Defaults shipped in the configmap**: `Identify` stays on Claude, and `Assess` / `Postmortem` / `OwnerUpdate` go to `opencode-go/gpt-5.6-luna`. `Consult` and `Ask` stay on Claude by default, because they are interactive and the tool loop deserves real use before it becomes the default path for a conversation.
4. ~~Live verification of the endpoint.~~ Resolved above. The key is in 1Password as `Opencode GO Planty Key` and reachable with `joey vault get`; it still needs adding to `~/secrets.sh` as `OPENCODE_API_KEY`, and to the `planty-secrets` k8s secret, which this change does.
