# syntax=docker/dockerfile:1

FROM node:22-alpine AS web-builder

WORKDIR /src

COPY web/package.json web/package-lock.json ./web/
RUN npm ci --prefix web

COPY CHANGELOG.md ./CHANGELOG.md
COPY web ./web
RUN npm run build --prefix web


FROM golang:1.26.8-alpine AS go-builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/internal/server/static/dist ./internal/server/static/dist
COPY --from=web-builder /src/web/node_modules ./web/node_modules
RUN go run ./scripts/notices -out /out/licenses

ARG TARGETOS=linux
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/techpedia \
    ./cmd/techpedia


FROM alpine:latest AS runtime

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S techpedia \
    && adduser -S techpedia -G techpedia \
    && mkdir -p /data/badger /data/auth /data/search /data/media/objects /data/media/temp \
    && chown -R techpedia:techpedia /data

COPY --from=go-builder /out/techpedia /usr/local/bin/techpedia
COPY --from=go-builder /out/licenses /usr/share/licenses/techpedia

USER techpedia

ENV TECHPEDIA_ADDR=0.0.0.0:8080
ENV TECHPEDIA_DATA_DIR=/data
ENV TECHPEDIA_PUBLIC_URL=http://localhost:8080
ENV TECHPEDIA_CSP_MODE=report-only

EXPOSE 8080

CMD ["techpedia"]
