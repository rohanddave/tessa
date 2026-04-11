FROM golang:1.25-alpine AS builder

RUN apk add --no-cache build-base pkgconf

ENV CGO_ENABLED=1

WORKDIR /app/indexing-service

COPY indexing-service/go.mod indexing-service/go.sum ./
COPY shared ../shared
COPY indexing-service/cmd ./cmd
COPY indexing-service/internal ./internal

RUN go build -tags musl -o /bin/indexing-service-consumer ./cmd/consumer

FROM alpine:3.20

RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /bin/indexing-service-consumer /usr/local/bin/indexing-service-consumer

USER appuser

CMD ["indexing-service-consumer"]
