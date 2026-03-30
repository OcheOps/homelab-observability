# Multi-stage build for small final image
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o throttle-engine ./main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata nmap
WORKDIR /app
COPY --from=builder /build/throttle-engine .
COPY config.yaml .

EXPOSE 8090
ENTRYPOINT ["./throttle-engine"]
