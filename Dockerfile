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

EXPOSE 6270

CMD ["./ypr-server"]