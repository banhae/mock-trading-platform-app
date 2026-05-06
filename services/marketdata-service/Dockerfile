FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o marketdata-service .

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/marketdata-service /marketdata-service

EXPOSE 8083

ENTRYPOINT ["/marketdata-service"]
