# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Install build dependencies.
RUN apk add --no-cache git ca-certificates

# Cache Go modules.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build the binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /bin/voiddb ./cmd/voiddb

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget tzdata

WORKDIR /app

COPY --from=builder /bin/voiddb /app/voiddb
COPY config.yaml /app/config.yaml

# Create data directories.
RUN mkdir -p /data/db /data/blob

EXPOSE 7700 7701

ENTRYPOINT ["/app/voiddb"]
CMD ["-config", "/app/config.yaml"]
