# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/turcompany ./cmd/web

FROM alpine:3.19
WORKDIR /opt/turcompany
RUN apk add --no-cache ca-certificates tzdata && update-ca-certificates
COPY --from=builder /out/turcompany /usr/local/bin/turcompany
COPY assets ./assets
COPY config ./config
ENV GIN_MODE=release
EXPOSE 4000
CMD ["/usr/local/bin/turcompany"]
