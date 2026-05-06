package main

import (
	"fmt"
	"os"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	DevMode     bool

	// Phase 2 — event bus (NATS JetStream).
	//
	// EventBusEnabled controls whether order-service attempts to connect to
	// NATS. When disabled, the service uses a no-op publisher so local and
	// test environments can run without a NATS dependency.
	EventBusEnabled bool
	NATSURL         string
	NATSStream      string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		if host := os.Getenv("DATABASE_HOST"); host != "" {
			dbPort := os.Getenv("DATABASE_PORT")
			if dbPort == "" {
				dbPort = "5432"
			}
			dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
				os.Getenv("DATABASE_USER"), os.Getenv("DATABASE_PASSWORD"),
				host, dbPort, os.Getenv("DATABASE_NAME"))
		}
	}

	stream := os.Getenv("NATS_STREAM")
	if stream == "" {
		stream = "ORDERS"
	}

	return Config{
		Port:        port,
		DatabaseURL: dbURL,
		JWTSecret:   os.Getenv("JWT_SECRET"),
		DevMode:     os.Getenv("DEV_MODE") == "true",

		EventBusEnabled: os.Getenv("EVENT_BUS_ENABLED") == "true",
		NATSURL:         os.Getenv("NATS_URL"),
		NATSStream:      stream,
	}
}
