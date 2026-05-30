# =============================================================================
# Stage 1: Build
# =============================================================================
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev git ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o inventory-service ./cmd/inventory-service

# =============================================================================
# Stage 2: Production
# =============================================================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl sqlite-libs

RUN addgroup -g 1000 inventory && adduser -u 1000 -G inventory -s /bin/sh -D inventory

WORKDIR /app
COPY --from=builder /app/inventory-service .
RUN mkdir -p /app/data && chown -R inventory:inventory /app

USER inventory

EXPOSE 8080 4317 4318

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENV GIN_MODE=release
ENV TZ=UTC

CMD ["./inventory-service"]
