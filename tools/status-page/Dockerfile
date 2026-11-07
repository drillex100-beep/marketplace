# syntax=docker/dockerfile:1.7
# The status page addon: its own code lives in this directory (tools/status-page), stdlib
# only. Build from the repository root:
#   docker buildx build --platform linux/amd64,linux/arm64 -f tools/status-page/Dockerfile .
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
WORKDIR /src
COPY tools/status-page/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/status-page .
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/status-page /status-page
USER 65532:65532
ENTRYPOINT ["/status-page"]
