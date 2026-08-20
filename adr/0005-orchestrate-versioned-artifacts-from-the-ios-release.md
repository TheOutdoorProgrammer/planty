# Orchestrate versioned artifacts from the iOS release

## Context and Problem Statement

Planty publishes an iOS app, a Go CLI, and a multi-architecture backend image from the same source tree.
The iOS workflow used Quill to create the version tag with the repository `GITHUB_TOKEN`, while a separate workflow expected that tag push to start the CLI and image release.
GitHub intentionally does not start new workflow runs for events created with a workflow's `GITHUB_TOKEN`, so the app shipped without a matching versioned backend image or CLI release.

Every release surface needs the same version without relying on recursive workflow events.
The backend image must also remain on a Linux runner because GitHub's macOS runners do not provide the Docker engine required by Buildx.

## Considered Options

- Keep the tag-triggered workflow and create tags with a personal access token or GitHub App token.
- Publish the CLI, IPA, and image directly through Quill in the macOS job.
- Let Quill publish the CLI and IPA, then pass its version output to the backend workflow as a reusable Linux job.

## Decision Outcome

Chosen: **let Quill publish the CLI and IPA, then pass its version output to the backend workflow as a reusable Linux job**.

The manually dispatched iOS workflow is the release orchestrator.
Quill calculates and creates one version, publishes the CLI and IPA, and exposes that version to a dependent job.
The dependent job calls the backend image workflow directly and publishes semver tags from the supplied version.
Ordinary pushes to `main` continue to invoke the same backend workflow independently for `main`, `latest`, and commit-SHA snapshot tags.

Using a stronger token to manufacture another workflow event was rejected because event recursion is implicit orchestration and adds a long-lived credential solely to bypass a GitHub safety boundary.
Publishing Docker from the macOS job was rejected because Quill's Docker publisher uses Buildx and the hosted macOS runner has no Docker engine.

### Consequences

Good:

- The iOS app, CLI, and backend image use one version selected by Quill.
- A release does not depend on GitHub turning a workflow-created tag into another workflow event.
- The backend image remains on a native Linux runner with multi-architecture Buildx support.
- The backend workflow remains reusable for both release images and per-commit snapshots.
- A dry run never invokes the publishing backend job.

Bad, and accepted:

- The backend image starts only after Quill has published the downloadable artifacts, so the three surfaces do not become available at exactly the same instant.
- If image publishing fails, the release already exists and must be completed by rerunning the failed backend job rather than cutting another version.
- The iOS workflow is now the sole entry point for versioned releases, even when a change affects only the backend.
