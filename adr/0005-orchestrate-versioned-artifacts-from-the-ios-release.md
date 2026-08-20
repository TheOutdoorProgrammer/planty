# Orchestrate versioned artifacts through a staged Quill release

## Context and Problem Statement

Planty publishes an iOS app, a Go CLI, and a multi-architecture backend image from the same source tree.
The iOS archive requires Xcode on macOS, while the backend image requires Docker on Linux.
A composite action cannot change runners between steps, and splitting the publishers into separate release jobs weakens Quill's guarantees around pre-tag validation, cleanup, and publisher ordering.

The Docker image also used to compile the Go binary again even though GoReleaser had already built the exact release binary.
That duplicated work and allowed the downloadable CLI and the binary inside the image to differ.

Every versioned release surface should come from one Quill release transaction, while platform-specific build work stays on the runner that supports it.
Pull requests and ordinary development validation belong in CI, not in the release workflow.

## Considered Options

- Keep the iOS release and backend image as separate workflows.
- Move all iOS build logic into Quill.
- Stage the signed IPA from macOS, then let Quill publish everything from Linux.

## Decision Outcome

Chosen: **stage the signed IPA from macOS, then let Quill publish everything from Linux**.

`.github/workflows/release.yml` is the only release workflow and is manually dispatched only.
Its macOS job owns Xcode testing, signing, archive, and export, then uploads `Planty.ipa` with `TheOutdoorProgrammer/quill/stage@v1`.
A dependent job calls Quill's `staged-release.yml@v1`, which downloads the IPA onto Ubuntu and runs GoReleaser, Fledge, and Docker in Quill's fixed order.

GoReleaser builds `dist/` before Docker runs.
The Dockerfile copies the matching Linux binary from that tree for each BuildKit target instead of invoking `go build` itself.
The image therefore contains the same binary Quill built for the release.

CI separately proves that handoff by running a GoReleaser snapshot and building the Dockerfile from its `dist/` tree on pull requests.
The release workflow does not publish snapshots or respond to branch pushes.

Using a stronger token to manufacture another workflow event was rejected because event recursion is implicit orchestration and adds a credential solely to bypass a GitHub safety boundary.
Moving repository-specific Xcode and signing commands into Quill was rejected because those steps belong to the application, not the release tool.

### Consequences

Good:

- The iOS app, CLI, and backend image use one version selected by Quill.
- Docker runs on Linux without separating it from GoReleaser's release transaction.
- Quill's Docker dry run happens before the tag is cut.
- A failed Docker publish participates in Quill's tag cleanup instead of leaving a partially completed release behind.
- The image packages the GoReleaser-built binary instead of rebuilding Go independently.
- There is one Planty release workflow to understand and maintain.
- CI owns non-release validation, so the release workflow contains release behavior only.

Bad, and accepted:

- The signed IPA crosses a workflow-artifact boundary before publishing.
- Building the Dockerfile directly now requires a GoReleaser `dist/` tree to exist first.
