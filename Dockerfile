FROM golang:1.26-alpine AS build

WORKDIR /src

# Cached separately so a source-only change does not re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/planty ./cmd/planty

# Claude Code ships a native musl build, so the judge backend costs one static
# binary rather than a Node runtime.
FROM alpine:3.22 AS claude

ARG CLAUDE_VERSION=stable

# Alpine drops old package versions, so pinning here breaks the build the week
# it lands rather than the year it matters.
# hadolint ignore=DL3018
RUN apk add --no-cache curl jq

SHELL ["/bin/ash", "-o", "pipefail", "-c"]
RUN set -eux; \
    case "$(uname -m)" in \
      aarch64) platform=linux-arm64-musl ;; \
      x86_64)  platform=linux-x64-musl ;; \
      *) echo "unsupported architecture $(uname -m)" >&2; exit 1 ;; \
    esac; \
    base=https://downloads.claude.ai/claude-code-releases; \
    version="$CLAUDE_VERSION"; \
    if [ "$version" = stable ] || [ "$version" = latest ]; then \
      version="$(curl -fsSL "$base/$version")"; \
    fi; \
    expected="$(curl -fsSL "$base/$version/manifest.json" \
      | jq -er --arg p "$platform" '.platforms[$p].checksum')"; \
    curl -fsSL -o /tmp/claude "$base/$version/$platform/claude"; \
    echo "$expected  /tmp/claude" | sha256sum -c -; \
    install -D -m 0755 /tmp/claude /out/claude; \
    /out/claude --version

FROM alpine:3.22

# The CLI shells out and reads its own config, so this cannot be distroless.
# bash specifically: the Bash tool runs bash, not busybox sh, and without it
# every command the model tries comes back as a broken shell.
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates libgcc libstdc++ bash \
    && adduser -D -u 65532 -h /home/planty planty

# On PATH because the model runs `planty agent ...` by name, and symlinked at
# the old absolute path so existing manifests and runbooks keep working.
COPY --from=build /out/planty /usr/local/bin/planty
RUN ln -s /usr/local/bin/planty /planty

COPY --from=claude /out/claude /usr/local/bin/claude

# Claude Code writes here every run, so it is a volume in the deployment and
# the image only guarantees the path exists and belongs to the user.
ENV HOME=/home/planty
RUN mkdir -p /home/planty/.claude && chown -R 65532:65532 /home/planty

# Numeric so Kubernetes runAsUser and the host both resolve it.
USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/planty"]
CMD ["serve"]
