FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o wallet-service .

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/wallet-service /wallet-service

EXPOSE 8082

ENTRYPOINT ["/wallet-service"]
