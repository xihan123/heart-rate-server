# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27
FROM golang:${GO_VERSION}-alpine3.24 AS build
WORKDIR /src

# SQLite uses CGO; build on the target platform against musl.
RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/server .

FROM alpine:latest AS final

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app \
    && mkdir -p /data \
    && chown app:app /data

WORKDIR /app
ENV DB_DSN=/data/heartrate.db

COPY --from=build /out/server /usr/local/bin/server
COPY templates ./templates
COPY static ./static

USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
