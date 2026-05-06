package main

import "os"

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port    string
	DevMode bool

	// Phase 2 — event bus (NATS JetStream) consumer wiring.
	EventBusEnabled bool
	NATSURL         string
	NATSStream      string
	NATSDurable     string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	stream := os.Getenv("NATS_STREAM")
	if stream == "" {
		stream = "ORDERS"
	}
	durable := os.Getenv("NATS_DURABLE")
	if durable == "" {
		durable = "marketdata-consumer"
	}

	return Config{
		Port:    port,
		DevMode: os.Getenv("DEV_MODE") == "true",

		EventBusEnabled: os.Getenv("EVENT_BUS_ENABLED") == "true",
		NATSURL:         os.Getenv("NATS_URL"),
		NATSStream:      stream,
		NATSDurable:     durable,
	}
}
