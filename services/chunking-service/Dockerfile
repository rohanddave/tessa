FROM golang:1.25-alpine AS builder

RUN apk add --no-cache build-base pkgconf

ENV CGO_ENABLED=1

WORKDIR /app/chunking-service

COPY chunking-service/go.mod chunking-service/go.sum ./
COPY shared ../shared
COPY chunking-service/cmd ./cmd
COPY chunking-service/internal ./internal

RUN go build -tags musl -o /bin/chunking-service-consumer ./cmd/consumer

FROM alpine:3.20

RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /bin/chunking-service-consumer /usr/local/bin/chunking-service-consumer

USER appuser

CMD ["chunking-service-consumer"]
