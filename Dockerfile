# Build stage
FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mizubot ./cmd/mizubot

# Runtime stage
FROM gcr.io/distroless/static-debian12

WORKDIR /
COPY --from=builder /app/mizubot /mizubot
COPY --from=builder /app/db/migrations /db/migrations

USER nonroot:nonroot

ENTRYPOINT ["/mizubot"]

