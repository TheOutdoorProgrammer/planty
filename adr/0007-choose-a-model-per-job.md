# Choose the model per job, and reach every model through one OpenAI-compatible harness

## Context and Problem Statement

Planty asks a model six questions, and until now it has asked all six of the same model.
`judge.Model` is a compile-time constant naming `claude-opus-5`, and the only escape is `PLANTY_JUDGE_MODEL`, read once at process start.
One process, one model, six jobs.

The six jobs are not alike.

| Job | Photographs | Effort | How often |
| --- | --- | --- | --- |
| `Assess`, the daily verdict | no | medium | once per plant per day, thirteen a day |
| `Identify`, what plant is this | **yes** | unset | when somebody photographs something |
| `Consult`, chat about one plant | **yes** | medium | interactive |
| `Ask`, chat about nothing yet | **yes** | medium | interactive |
| `Postmortem`, what killed it | no | high | when a plant dies |
| `OwnerUpdate`, draft a note to an owner | no | medium | interactive |

`Assess` is a cheap structured verdict over sparse numbers, run on a cron, and it is the overwhelming majority of the volume.
`Identify` is fine-grained visual classification, it is rare, and being confidently wrong about it is the most expensive mistake the system can make: its own system prompt says an empty list beats a guessed name, because somebody waters a plant based on the answer.
Paying Opus prices for the first and accepting a weaker model for the second is exactly backwards, and one constant cannot express the difference.

A subscription to OpenCode Go supplies twenty-three models behind an OpenAI-compatible endpoint at `https://opencode.ai/zen/go/v1`, authenticated with an API key, metered against flat monthly caps rather than billed per request.
That makes the choice of model a genuine dial rather than a bill, and it makes the question "which model for which job" worth answering per job instead of once.

Two facts constrain any answer.
Every one of the six call sites passes a JSON schema and unmarshals the validated result, so a model without structured output cannot serve any job at all.
Three of the six attach or offer photographs, so a model without vision cannot serve those three.
In the Go roster those two filters together eliminate most of the catalogue, and a picker that lets someone select `glm-5.3` for `Identify` is a picker that produces a runtime failure on a phone in a garden centre.

## Considered Options

- **Keep one model for everything.**
- **A second environment variable per job.**
- **Persist a model choice per job, and reach every provider through one OpenAI-compatible harness.**
- **Shell out to `opencode run` the way the Claude backend shells out to `claude`.**

## Decision Outcome

Chosen: **persist a model choice per job, and reach every provider through one OpenAI-compatible harness.**

Model moves from the backend to the `Request`.
It was frozen into `apiBackend.model` and `cliBackend.model` at construction, which is what made one `Judge` mean one model.
`Effort` and `Acting` already vary per call site and already live on `Request`; model belongs beside them, and putting it there means the six call sites keep sharing the single `Judge` on the server rather than needing six of them.

A provider is a triple of kind, base URL and credential.
`anthropic` and `claude-cli` stay exactly as they are.
The new `openai` kind is instantiable more than once, so the same code reaches OpenCode Go, OpenCode Zen, OpenAI proper, LiteLLM or a local Ollama without another backend being written.
That generality is nearly free (it is one struct field), and it is the difference between buying into one vendor's subscription and buying into a wire format that every vendor already speaks.

`/chat/completions` is the harness's wire format.
OpenCode Go actually routes three shapes: `/responses` for `gpt-5.6-luna`, `grok-4.5` and `muse-spark-1.2`, `/messages` for the three MiniMax models, and `/chat/completions` for everything else.
The MiniMax models are text-only and lack structured output, so Planty could never use them whatever the wire format, and their exclusion costs nothing.

An empty settings table means the environment defaults still apply, so the deployment keeps working with no configuration and the feature is additive.

The picker is gated on capability rather than trusting the person holding the phone.
The catalogue endpoint annotates each model with vision, structured output and effort support, and a job that offers photographs will not offer a model that cannot see.

### Consequences

Good:

- The expensive judgement and the cheap one stop costing the same, and each job can be moved without redeploying.
- Vision work and volume work can sit on different models and different providers at the same time, which is the arrangement the evidence actually argues for: keep `Identify` on Claude, move `Assess` to something cheap.
- Any OpenAI-compatible endpoint becomes reachable, so the subscription is replaceable without touching Planty again.
- `Outcome` gains the model that answered, which fixes a lie described below.

Bad:

- Planty grows its first persisted settings table, its first settings routes, and a settings surface on the phone that did not exist. Every knob until now was an environment variable read at start-up, and that simplicity is being spent deliberately.
- Six jobs times a model each is six ways to misconfigure the system, and a bad choice now fails at runtime in front of a person rather than at deploy time. Capability gating covers the mechanical failures (no vision, no schema) and covers nothing about a model simply being bad at the job.
- Structured output on the OpenAI side is `response_format: json_schema`, which providers implement with varying strictness. The Anthropic path enforces it natively. Two paths with different guarantees now produce the same `Outcome`, and a schema violation will surface as a decode error rather than a refusal.
- The tool-using jobs stay on Claude for now. `Consult` and `Ask` grant `Bash(planty agent *)` plus fetching from an allowlist, and that agentic loop lives in the Claude Code CLI backend. The harness answers in one shot; it does not run tools. Selecting an OpenAI model for those two jobs must therefore be refused rather than silently degraded.

### Fixing the model that answered

`internal/job/daily.go` stamped `judge.Model` (the compile-time constant) into every verdict's `Evidence.ModelVersion`, and that value travels to the phone and is rendered as "Model …" on the why-Planty-thinks-this screen.
It has been wrong since `PLANTY_JUDGE_MODEL` existed, because the constant does not change when the override does.
Per-job models would turn a stale label into a confident falsehood on every verdict.

`Backend.Judge` now reports the model that answered on the `Outcome`, and the daily job stamps that instead of the constant.
This is not scope creep — there is no correct way to record which model produced a verdict once six jobs can use six models, and the provenance has to be fixed in the same change that makes it matter.
