FROM golang:1.23-bullseye AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -o gamebot .

# runtime
FROM debian:bullseye-slim

WORKDIR /app
COPY --from=builder /app/gamebot .

# устанавливаем корневые сертификаты
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

CMD ["./gamebot"]