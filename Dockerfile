# ── Frontend build stage ──────────────────────────────────────────────────────
# The web UI is embedded in the binary (web/embed.go). web/static is
# gitignored, so without this stage the published image served only a
# "frontend not built" page.
FROM node:22-alpine AS frontend

WORKDIR /app

# Package files first so the dependency layer caches until they change.
COPY web/ui/package.json web/ui/package-lock.json ./web/ui/
RUN cd web/ui && npm ci

# The rest of the frontend, including the vendored shared components
# under src/shared/ (copied from beacon-stack/web-shared).
COPY web/ui/ ./web/ui/
RUN cd web/ui && npm run build
# Output lands in /app/web/static/

# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overwrite web/static/ with the freshly built frontend.
COPY --from=frontend /app/web/static/ ./web/static/
RUN CGO_ENABLED=0 go build -o /pulse ./cmd/pulse

# ── Runtime stage ──────────────────────────────────────────────────────��─────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -h /app pulse

WORKDIR /app
COPY --from=build /pulse .

USER pulse

EXPOSE 9696

VOLUME ["/config"]

# Defaults for the published image. The database is a SQLite file under
# /config; the API key is generated on first run and stored in that DB.
# Override any of these via the container's `environment:` block — env
# always beats Dockerfile ENV.
ENV PULSE_DATABASE_PATH=/config/pulse.db \
    PULSE_SERVER_EXTERNAL_URL=http://pulse:9696 \
    PULSE_LOG_LEVEL=info \
    TZ=UTC

ENTRYPOINT ["./pulse"]
