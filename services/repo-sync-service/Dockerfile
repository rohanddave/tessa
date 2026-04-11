FROM golang:1.25-alpine AS builder

RUN apk add --no-cache build-base pkgconf

ENV CGO_ENABLED=1

WORKDIR /app/repo-sync-service

COPY repo-sync-service/go.mod repo-sync-service/go.sum ./
COPY shared ../shared
COPY repo-sync-service/cmd ./cmd
COPY repo-sync-service/internal ./internal

RUN go build -tags musl -o /bin/repo-sync-service-api ./cmd/api
RUN go build -tags musl -o /bin/repo-sync-service-consumer ./cmd/consumer

FROM alpine:3.20

RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /bin/repo-sync-service-api /usr/local/bin/repo-sync-service-api
COPY --from=builder /bin/repo-sync-service-consumer /usr/local/bin/repo-sync-service-consumer

EXPOSE 8081

USER appuser

CMD ["repo-sync-service-api"]
