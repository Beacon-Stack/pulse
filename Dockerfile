# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /pulse ./cmd/pulse

# ── Runtime stage ──────────────────────────────────────────────────────��─────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -h /app pulse

WORKDIR /app
COPY --from=build /pulse .

USER pulse

EXPOSE 9696

VOLUME ["/config"]

# Beacon-stack-shaped defaults — the published image expects to land in a
# compose with a service named `postgres` and Docker secrets mounted under
# /run/secrets/. For standalone use, override these via the container's
# `environment:` block; env always beats Dockerfile ENV. See
# docker-compose.yml for the standalone recipe.
ENV PULSE_DATABASE_DRIVER=postgres \
    PULSE_DATABASE_DSN="postgres://pulse@postgres:5432/pulse_db?sslmode=disable" \
    PULSE_DATABASE_PASSWORD_FILE=/run/secrets/pulse.txt \
    PULSE_AUTH_API_KEY_FILE=/run/secrets/pulse-api-key.txt \
    PULSE_SERVER_EXTERNAL_URL=http://pulse:9696 \
    PULSE_LOG_LEVEL=info \
    TZ=UTC

ENTRYPOINT ["./pulse"]
