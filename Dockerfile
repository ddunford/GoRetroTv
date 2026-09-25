# The emulator is a single static binary and the firmware is NOT baked in: the images are Pace's,
# not ours (firmware/MANIFEST.md), so a running container is given them as a mounted volume and a
# published image stays inert without them.

# Build the checked TypeScript source in an isolated stage. No host-generated dist files enter the
# image, so a stale local `web/dist` cannot silently become the page served in production.
FROM node:24.19.0-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS web-builder
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY tsconfig.json ./
COPY web/*.ts ./web/
RUN npm run build:web

# Linux/amd64 image manifest, resolved from the official Docker Hub registry.
FROM golang:1.27.1@sha256:b475798fb16158e6c38e8b5ca2d870fbeaa8b7fec0fc8ec64b3dc20966040635 AS builder
WORKDIR /src

# Keep the approved WebSocket dependency in its own cache layer and verify it
# against the committed module checksums before compiling the binary.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# CGO_ENABLED=0 keeps the emulator core independent of the host decoder and its shared libraries.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/ddunford/goretrotv/internal/version.Version=${VERSION} \
      -X github.com/ddunford/goretrotv/internal/version.Commit=${COMMIT} \
      -X github.com/ddunford/goretrotv/internal/version.Date=${BUILD_DATE}" \
    -o /out/ ./cmd/...

# FFmpeg is an explicit subprocess boundary, not linked into the emulator. Debian 13 supplies both
# ffmpeg and ffprobe from one maintained package while the Go binary itself remains static/no-CGo.
FROM debian:13.1-slim@sha256:a347fd7510ee31a84387619a492ad6c8eb0af2f2682b916ff3e643eb076f925a AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/goretrotv /goretrotv
COPY web/index.html web/styles.css web/favicon.svg /web/
COPY --from=web-builder /src/web/dist /web/dist
# oraclecmp travels with the emulator rather than being a separate developer-only build, because
# it answers a question about THIS binary: whether the checkpoint stream this build produced
# matches the browser oracle's (SPEC FR-6). Shipping it here removes the version-skew question -
# "was that stream produced by this build?" - which is exactly the kind of doubt that turns a
# divergence into an afternoon. Run it with `--entrypoint /oraclecmp`; it is inert otherwise, and
# it is inert unless selected as the container entrypoint.
COPY --from=builder /out/oraclecmp /oraclecmp
USER 65532:65532
EXPOSE 8099
ENTRYPOINT ["/goretrotv"]
