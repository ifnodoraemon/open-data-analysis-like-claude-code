# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/server .

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /app/server .
RUN mkdir -p uploads data/cache

EXPOSE 8080

CMD ["./server"]
