# The emulator is a single static binary and the firmware is NOT baked in: the images are Pace's,
# not ours (firmware/MANIFEST.md), so a running container is given them as a mounted volume and a
# published image stays inert without them.

# Linux/amd64 image manifest, resolved from the official Docker Hub registry.
FROM golang:1.27.1@sha256:b475798fb16158e6c38e8b5ca2d870fbeaa8b7fec0fc8ec64b3dc20966040635 AS builder
WORKDIR /src

# The module has no external dependencies, so there is nothing to pre-download; copying go.mod
# first still keeps the module graph in its own cache layer.
COPY go.mod ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# CGO_ENABLED=0 is mandatory for the distroless static base, which has no libc. It is also the
# recorded decision for the core: no CGo (CLAUDE.md -> Stack).
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/ddunford/goretrotv/internal/version.Version=${VERSION} \
      -X github.com/ddunford/goretrotv/internal/version.Commit=${COMMIT} \
      -X github.com/ddunford/goretrotv/internal/version.Date=${BUILD_DATE}" \
    -o /out/ ./cmd/...

# Linux/amd64 image manifest from the distroless registry. Debian 13 is the supported runtime line.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:2293b36c7c9082bf4115aab724b4d2cddec82c8eba39bf27ac0517e159acf150 AS runtime
COPY --from=builder /out/goretrotv /goretrotv
# oraclecmp travels with the emulator rather than being a separate developer-only build, because
# it answers a question about THIS binary: whether the checkpoint stream this build produced
# matches the browser oracle's (SPEC FR-6). Shipping it here removes the version-skew question -
# "was that stream produced by this build?" - which is exactly the kind of doubt that turns a
# divergence into an afternoon. Run it with `--entrypoint /oraclecmp`; it is inert otherwise, and
# the base has no shell for it to be reachable from.
COPY --from=builder /out/oraclecmp /oraclecmp
USER nonroot:nonroot
EXPOSE 8099
ENTRYPOINT ["/goretrotv"]
