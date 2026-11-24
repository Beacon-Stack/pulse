# Frontend build stage
# The web UI is embedded in the binary (web/embed.go). web/static is
# gitignored, so without this stage the published image served only a
# "frontend not built" page.
# The UI is plain JS/CSS, the same for every platform, so build it once
# on the runner's own platform instead of again under arm64 emulation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend

WORKDIR /app

# Package files first so the dependency layer caches until they change.
COPY web/ui/package.json web/ui/package-lock.json ./web/ui/
RUN cd web/ui && npm ci

# The rest of the frontend, including the vendored shared components
# under src/shared/ (copied from beacon-stack/web-shared).
COPY web/ui/ ./web/ui/
RUN cd web/ui && npm run build
# Output lands in /app/web/static/

# Build stage
# Go cross-compiles: build natively and target the image's platform with
# GOOS/GOARCH. Emulating arm64 made this step ~10x slower (466 s vs 48 s).
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overwrite web/static/ with the freshly built frontend.
COPY --from=frontend /app/web/static/ ./web/static/
# CI passes the commit as VERSION; the update check compares it with the
# newest published build.
ARG VERSION=dev
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
      -ldflags "-X github.com/beacon-stack/pulse/internal/version.Version=${VERSION} \
                -X github.com/beacon-stack/pulse/internal/version.BuildTime=${BUILD_TIME} -s -w" \
      -o /pulse ./cmd/pulse

# Files for the final image, which has no shell to make them: the config
# folder (owned by the app's user) and a "wget" link to the app, so
# compose health checks written for the old image keep working.
RUN mkdir -p /rootfs/config /rootfs/usr/bin && ln -s /app/pulse /rootfs/usr/bin/wget


# Final stage
# Distroless: the app, CA certificates, timezone data and /tmp. No shell,
# no package manager, nothing to patch. Pinned by digest; bump on purpose.
FROM gcr.io/distroless/static-debian12@sha256:d75cdd72874d4790092fcb1b058493ecf6bb5bf2b2b897045b00ff01d91843f2
WORKDIR /app

COPY --from=build /pulse /app/pulse
COPY --from=build /rootfs/usr/bin/ /usr/bin/
COPY --from=build --chown=1000:1000 /rootfs/config /config

ENV PULSE_DATABASE_PATH=/config/pulse.db \
    PULSE_SERVER_EXTERNAL_URL=http://pulse:9696 \
    PULSE_LOG_LEVEL=info \
    TZ=UTC \
    HOME=/config

VOLUME ["/config"]

EXPOSE 9696

# The same user ID as the old Alpine image, so existing /config folders
# keep working.
USER 1000:1000

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 CMD ["/app/pulse", "healthcheck"]

ENTRYPOINT ["/app/pulse"]
