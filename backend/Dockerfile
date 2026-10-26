# syntax=docker/dockerfile:1

# --- Build-Stage -----------------------------------------------------------
FROM golang:1.25-alpine AS build

ARG APP_VERSION=dev

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${APP_VERSION}" \
        -o /out/server ./cmd/server

# --- Runtime-Stage ---------------------------------------------------------
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -u 10001 -G app -h /app app

COPY --from=build /out/server /usr/local/bin/server

USER 10001:10001
WORKDIR /app
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/server"]
