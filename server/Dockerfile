# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/server .

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata pandoc \
    && adduser -D -H -h /app appuser

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /app/server .
RUN mkdir -p data/cache data/metadata data/storage data/tmp \
    && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1

CMD ["./server"]
