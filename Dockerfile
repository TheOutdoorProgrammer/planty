# Codex's native musl build keeps subscription authentication in the supported
# app-server without adding a Node runtime.
FROM alpine:3.22 AS codex

ARG CODEX_VERSION=0.154.0

# Alpine drops old package versions, so pinning here breaks the build the week
# it lands rather than the year it matters.
# hadolint ignore=DL3018
RUN apk add --no-cache curl

SHELL ["/bin/ash", "-o", "pipefail", "-c"]
RUN set -eux; \
    case "$(uname -m)" in \
      aarch64) platform=aarch64-unknown-linux-musl; expected=583b48df32804213bdcd338c2e5adb06b34340821fa757a726cc0a524fa33c27; host_expected=20aefa302c2022b496e32911bf954a5f76c7fd749c6bdb9fbd711e32b66dcbfa ;; \
      x86_64) platform=x86_64-unknown-linux-musl; expected=d7e18b2597ae8f242f5f31ee9e90deef48dbc9edd634d9868fb6435d08c07f02; host_expected=a68df7cca23c6da7cde175677df7de61c73a234add1333a1254b86d641af01f7 ;; \
      *) echo "unsupported architecture $(uname -m)" >&2; exit 1 ;; \
    esac; \
    base="https://github.com/openai/codex/releases/download/rust-v$CODEX_VERSION"; \
    curl -fsSL -o /tmp/codex.tar.gz "$base/codex-$platform.tar.gz"; \
    echo "$expected  /tmp/codex.tar.gz" | sha256sum -c -; \
    tar -xzf /tmp/codex.tar.gz -C /tmp; \
    install -D -m 0755 "/tmp/codex-$platform" /out/codex; \
    curl -fsSL -o /tmp/host.tar.gz "$base/codex-code-mode-host-$platform.tar.gz"; \
    echo "$host_expected  /tmp/host.tar.gz" | sha256sum -c -; \
    tar -xzf /tmp/host.tar.gz -C /tmp; \
    install -D -m 0755 "/tmp/codex-code-mode-host-$platform" /out/codex-code-mode-host; \
    /out/codex --version; \
    /out/codex app-server --help >/dev/null; \
    /out/codex-code-mode-host --help >/dev/null

FROM alpine:3.22 AS symbols

# hadolint ignore=DL3018
RUN apk add --no-cache llvm20

FROM alpine:3.22

# hadolint ignore=DL3018
RUN apk add --no-cache bubblewrap ca-certificates libgcc libstdc++ llvm20-libs libcurl \
    && adduser -D -u 65532 -h /home/planty planty

COPY --from=symbols /usr/lib/llvm20/bin/llvm-symbolizer /usr/local/bin/llvm-symbolizer
RUN llvm-symbolizer --version

# GoReleaser already built the exact binary being published. BuildKit sets
# TARGETARCH for each platform, and GoReleaser includes the Go architecture
# variant in its per-target dist directory (for example amd64_v1 or arm64_v8.0).
ARG TARGETARCH
ARG VERSION
ARG COMMIT
COPY dist/planty_linux_${TARGETARCH}_*/planty /usr/local/bin/planty

# A snapshot dist directory can otherwise survive in BuildKit's cache while
# the image receives the new release tag and labels.
RUN set -eu; \
    if [ -n "${VERSION:-}" ]; then \
      output="$(/usr/local/bin/planty version)"; \
      details="${output#* }"; \
      actual_version="${details%% *}"; \
      actual_commit="${details#* }"; \
      actual_commit="${actual_commit#(}"; \
      actual_commit="${actual_commit%)}"; \
      test "$actual_version" = "${VERSION#v}"; \
      test "$actual_commit" = "$(printf '%.7s' "$COMMIT")"; \
    fi

# On PATH because the model runs `planty agent ...` by name, and symlinked at
# the old absolute path so existing manifests and runbooks keep working.
RUN ln -s /usr/local/bin/planty /planty

COPY --from=codex /out/codex /usr/local/bin/codex
COPY --from=codex /out/codex-code-mode-host /usr/local/bin/codex-code-mode-host

ENV HOME=/home/planty
RUN mkdir -p /home/planty/.codex && chown -R 65532:65532 /home/planty

# Numeric so Kubernetes runAsUser and the host both resolve it.
USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/planty"]
CMD ["serve"]
