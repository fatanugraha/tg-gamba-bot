# Build stage
FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -o casino .

# Runtime stage using distroless
FROM gcr.io/distroless/cc-debian12

WORKDIR /app

COPY --from=builder /app/casino /app/casino

CMD ["/app/casino"]
