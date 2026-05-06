FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o auth-service .

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/auth-service /auth-service

EXPOSE 8081

ENTRYPOINT ["/auth-service"]
