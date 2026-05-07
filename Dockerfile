FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o ypr-server .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/ypr-server .
ENV DOCKER=true
ENV PORT=8080
ENV DB_PATH=/app/data/ypr.db
ENV CLIENT_SECRET=/app/client_secret.json
EXPOSE 8080
CMD ["./ypr-server"]
