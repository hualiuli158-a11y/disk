# Build the API and migration tool with the Go version required by go.mod.
FROM golang:1.26.6-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/disk . && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# Only the binaries and runtime certificates go into the final image.
FROM alpine:3.24

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN addgroup -S diskapp && adduser -S -G diskapp diskapp && \
    mkdir -p /app/data && chown diskapp:diskapp /app/data

WORKDIR /app
COPY --from=build /out/disk /app/disk
COPY --from=build /out/migrate /app/migrate

ENV APP_ENV=production \
    PORT=8080 \
    STORAGE_ROOT=/app/data

USER diskapp
EXPOSE 8080

# Production mode requires a private listen address.
# Use the container IP unless LISTEN is supplied explicitly.
ENTRYPOINT ["/bin/sh", "-ec", "if [ -z \"${LISTEN:-}\" ]; then set -- $(hostname -i); export LISTEN=\"$1\"; fi; exec /app/disk"]
