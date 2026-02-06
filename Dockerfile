# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/turcompany ./cmd/web

FROM alpine:3.19
WORKDIR /opt/turcompany
RUN apk add --no-cache ca-certificates tzdata postgresql-client && update-ca-certificates
RUN addgroup -S app && adduser -S -G app -h /opt/turcompany app
RUN mkdir -p /opt/turcompany/files /opt/turcompany/config && chown -R app:app /opt/turcompany
COPY --from=builder /out/turcompany /usr/local/bin/turcompany
COPY assets ./assets
COPY bin/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
USER app
ENV GIN_MODE=release
EXPOSE 4000
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/turcompany"]
