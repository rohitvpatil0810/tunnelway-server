# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25.3

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /app

RUN apk add --no-cache ca-certificates

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build the server binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/tunnelway-server ./cmd/tunnelway-server

FROM scratch AS runtime

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/tunnelway-server /tunnelway-server

# Container-friendly bind address; can be overridden at runtime.
ENV TUNNELWAY_LISTEN_ADDR=0.0.0.0:6000

EXPOSE 6000
USER 65532:65532
ENTRYPOINT ["/tunnelway-server"]
