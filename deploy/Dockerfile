# syntax=docker/dockerfile:1

FROM golang:1.27.1 AS dependencies

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

FROM dependencies AS build

COPY cmd ./cmd
COPY config ./config
COPY internal ./internal
COPY pkg ./pkg

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/go-auth \
    ./cmd/server

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/go-auth /go-auth

USER 65532:65532

EXPOSE 4040 5050

ENTRYPOINT ["/go-auth"]
