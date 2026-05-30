# =============================================================================
# Stage 1: Build
# =============================================================================
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev git ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o lawrence ./cmd/all-in-one

# =============================================================================
# Stage 2: Production
# =============================================================================
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata curl sqlite-libs

RUN addgroup -g 1000 lawrence && adduser -u 1000 -G lawrence -s /bin/sh -D lawrence

WORKDIR /app
COPY --from=builder /app/lawrence .
RUN mkdir -p /app/data && chown -R lawrence:lawrence /app

USER lawrence

EXPOSE 8080 4320 4317 4318

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENV GIN_MODE=release
ENV TZ=UTC

CMD ["./lawrence"]
