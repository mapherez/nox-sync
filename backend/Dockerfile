# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /out/nox-sync ./cmd/nox-sync

FROM alpine:3.23

RUN adduser -D -H -u 10001 noxsync \
  && mkdir -p /data \
  && chown -R noxsync:noxsync /data

USER noxsync

EXPOSE 8080

ENV NOX_SYNC_ADDR=:8080
ENV NOX_SYNC_DATA_DIR=/data

COPY --from=build /out/nox-sync /usr/local/bin/nox-sync

ENTRYPOINT ["/usr/local/bin/nox-sync"]
