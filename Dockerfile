FROM node:24-alpine AS ts-builder
WORKDIR /app
COPY package.json package-lock.json tsconfig.json ./
RUN npm ci
COPY ts ./ts
RUN npx tsc

FROM golang:1.26.5-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o ypr-server .

FROM alpine:latest
RUN apk add --no-cache sqlite-libs ca-certificates

WORKDIR /app

ENV DOCKER=true

COPY --from=builder /app/ypr-server .
COPY --from=builder /app/static ./static
COPY --from=ts-builder /app/static/js ./static/js

RUN mkdir -p /db /app/media /config && chmod 777 /db /app/media /config

# OpenTelemetry configuration (all optional):
#   OTEL_EXPORTER_OTLP_ENDPOINT   - OTLP collector endpoint (e.g. http://otel-collector:4318)
#   OTEL_EXPORTER_OTLP_PROTOCOL   - Transport protocol: "grpc" (default) or "http"
#   OTEL_EXPORTER_OTLP_HEADERS    - JSON headers for OTLP export (e.g. {"Authorization":"Bearer xxx"})
#   OTEL_SERVICE_NAME             - Service name (default: youtube-playlist-randomizer)
#   OTEL_TRACES_SAMPLER           - Sampler: always_on, always_off, traceidratio,
#                                    parentbased_always_on, parentbased_always_off,
#                                    parentbased_traceidratio (default)
#   OTEL_TRACES_SAMPLER_ARG       - Sample ratio for traceidratio sampler (0.0-1.0)
#   OTEL_RESOURCE_ATTRIBUTES      - Comma-separated key=value resource attributes
#
# Disable specific signals by setting the corresponding env var to "" or "false".
# See internal/telemetry/telemetry.go Settings struct for programmatic control.
EXPOSE 6270

CMD ["./ypr-server"]