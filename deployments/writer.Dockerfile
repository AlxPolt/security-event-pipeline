FROM golang:1.26.0-trixie AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download && go mod verify

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-w -s -extldflags '-static' -X main.version=1.0.0" \
    -tags netgo \
    -o /out/writer \
    ./cmd/writer

FROM gcr.io/distroless/static-debian12:nonroot AS production

COPY --from=builder /out/writer /writer

ENTRYPOINT ["/writer"]
