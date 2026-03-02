# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates && update-ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# pdfcpu (нужен для merge original.pdf + sign_page.pdf)
RUN go install github.com/pdfcpu/pdfcpu/cmd/pdfcpu@v0.9.1

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build -trimpath -ldflags="-s -w" -o /out/turcompany ./cmd/web


FROM alpine:3.19
WORKDIR /opt/turcompany

# runtime deps: LO + fonts + tz + certs
RUN apk add --no-cache \
    ca-certificates tzdata postgresql-client \
    libreoffice font-noto fontconfig ttf-dejavu \
  && update-ca-certificates

# (опционально) если в коде бинарь называется "libreoffice", а в alpine реально "soffice"
# обычно хватает указать в config binary=soffice, но симлинк тоже ок.
RUN ln -sf /usr/bin/soffice /usr/local/bin/libreoffice || true

# user
RUN addgroup -S app && adduser -S -G app -h /opt/turcompany app

# dirs + права
RUN mkdir -p /opt/turcompany/files/pdf /opt/turcompany/files/docx /opt/turcompany/files/excel /opt/turcompany/config \
  && chown -R app:app /opt/turcompany

# binaries
COPY --from=builder /out/turcompany /usr/local/bin/turcompany
COPY --from=builder /go/bin/pdfcpu /usr/local/bin/pdfcpu

# assets + entrypoint
COPY assets ./assets
COPY bin/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN sed -i 's/\r$//' /usr/local/bin/entrypoint.sh \
  && chmod +x /usr/local/bin/entrypoint.sh \
  && chmod +x /usr/local/bin/pdfcpu || true

USER app
ENV GIN_MODE=release
EXPOSE 4000

ENTRYPOINT ["sh", "/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/turcompany"]