FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o ingestor ./cmd/ingestor


FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /build/ingestor .
COPY --from=builder /build/cmd/ingestor/config.example.yaml ./config.yaml

ENTRYPOINT ["/app/ingestor"]

CMD ["-c", "/app/config.yaml"]
