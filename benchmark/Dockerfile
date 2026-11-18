# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY benchmark/go.mod benchmark/go.sum ./benchmark/
RUN cd benchmark && go mod download

COPY internal ./internal
COPY benchmark ./benchmark

RUN cd benchmark \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/loadgen ./cmd/loadgen

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/seed /usr/local/bin/seed
COPY --from=builder /out/loadgen /usr/local/bin/loadgen
