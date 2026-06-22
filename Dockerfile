FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN go build -o servtrace main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/servtrace .
EXPOSE 8090
ENTRYPOINT ["./servtrace"]
