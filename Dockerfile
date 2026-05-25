# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
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
