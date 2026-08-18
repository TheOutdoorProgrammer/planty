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

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/planty /planty

# Numeric so Kubernetes runAsUser and the host both resolve it.
USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/planty"]
CMD ["serve"]
