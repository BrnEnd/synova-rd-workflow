FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=$(cat VERSION 2>/dev/null || echo dev)" -o synova-rd-workflow ./cmd/api

# ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/synova-rd-workflow .

EXPOSE 8080
ENTRYPOINT ["./synova-rd-workflow"]
