# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /configurarr ./cmd/configurarr

# ── Runtime stage ──────────────────────────────────────────────────────��─────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -h /app configurarr

WORKDIR /app
COPY --from=build /configurarr .

USER configurarr

EXPOSE 9696

VOLUME ["/app/data"]

ENTRYPOINT ["./configurarr"]
