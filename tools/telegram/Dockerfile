# syntax=docker/dockerfile:1.7
# The official Telegram bot is Scilla's own code: cmd/scilla-telegram of the scilla release
# this tool's version names. Build from the repository root (CI passes VERSION from
# adapter.json; the image takes nothing from this repository):
#   docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=0.5.2.1 \
#     -f tools/telegram/Dockerfile .
# Go cross-compiles on the build platform; only the final stage is the target's.
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
# The scilla release to build, without the "v": the tool's version in adapter.json, so one
# version is always the same source.
ARG VERSION
RUN test -n "$VERSION" || { echo "VERSION (the scilla release) is required"; exit 1; }
WORKDIR /src
RUN git clone --depth 1 --branch "v${VERSION}" https://github.com/drillex100-beep/scilla.git . && \
    git log -1 --format="scilla v${VERSION} at %H"
RUN --mount=type=cache,target=/go/pkg/mod go mod download
# Filled in by BuildKit for each platform. A default here would win over it and put
# amd64 binaries into the arm64 image.
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/scilla-telegram ./cmd/scilla-telegram
# The binary must be the image's own architecture: the ELF machine field says so.
RUN case "$TARGETARCH" in amd64) want="3e 00" ;; arm64) want="b7 00" ;; *) want="" ;; esac; \
    got=$(od -A n -t x1 -j 18 -N 2 /out/scilla-telegram | tr -s " " | sed "s/^ //"); \
    if [ -n "$want" ] && [ "$got" != "$want" ]; then echo "the bot is not built for $TARGETARCH (ELF machine $got)"; exit 1; fi

# Static binary and CA certificates, no shell; runs as 65532 and writes no files (its state
# is in the panel's KV), so a read-only root file system is fine.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/scilla-telegram /scilla-telegram
USER 65532:65532
ENTRYPOINT ["/scilla-telegram"]
