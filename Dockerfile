FROM node:24-alpine AS ts-builder

WORKDIR /app
COPY package.json package-lock.json tsconfig.json ./
RUN npm ci
COPY ts ./ts
RUN npx tsc

FROM golang:1.26.3-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o ypr-server .

FROM alpine:latest
RUN apk add --no-cache ca-certificates

WORKDIR /app

ENV DOCKER=true

COPY --from=builder /app/ypr-server .
COPY --from=builder /app/static ./static
COPY --from=ts-builder /app/static/js ./static/js

RUN mkdir -p /db && chmod 777 /db
RUN mkdir -p /app/media && chmod 777 /app/media

EXPOSE 6270

CMD ["./ypr-server"]