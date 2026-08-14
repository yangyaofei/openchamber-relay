# syntax=docker/dockerfile:1
# OpenChamber relay image (Go WebSocket relay).

FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o relay .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/relay /usr/local/bin/relay

EXPOSE 8080

USER nobody
CMD ["relay"]
