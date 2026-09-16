# The emulator is a single static binary and the firmware is NOT baked in: the images are Pace's,
# not ours (firmware/MANIFEST.md), so a running container is given them as a mounted volume and a
# published image stays inert without them.

FROM golang:1.22 AS builder
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
RUN CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local go build \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/ddunford/goretrotv/internal/version.Version=${VERSION} \
      -X github.com/ddunford/goretrotv/internal/version.Commit=${COMMIT} \
      -X github.com/ddunford/goretrotv/internal/version.Date=${BUILD_DATE}" \
    -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=builder /out/goretrotv /goretrotv
USER nonroot:nonroot
EXPOSE 8099
ENTRYPOINT ["/goretrotv"]
