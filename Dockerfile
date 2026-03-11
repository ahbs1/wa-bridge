# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o wa-bridge main.go

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates ffmpeg

COPY --from=builder /build/wa-bridge /app/wa-bridge
COPY --from=builder /build/public /app/public

WORKDIR /app
RUN mkdir -p /app/data

EXPOSE 3000
VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3000/health || exit 1

CMD ["./wa-bridge"]
