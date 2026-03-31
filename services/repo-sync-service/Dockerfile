FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /bin/repo-sync-service-api ./cmd/api
RUN go build -o /bin/repo-sync-service-consumer ./cmd/consumer

FROM alpine:3.20

RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /bin/repo-sync-service-api /usr/local/bin/repo-sync-service-api
COPY --from=builder /bin/repo-sync-service-consumer /usr/local/bin/repo-sync-service-consumer

EXPOSE 8081

USER appuser

CMD ["repo-sync-service-api"]
