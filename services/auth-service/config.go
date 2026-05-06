package main

import (
	"fmt"
	"os"
	"time"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port        string
	JWTSecret   string
	TokenExpiry time.Duration
	DevMode     bool
}

// LoadConfig reads configuration from environment variables.
// When DEV_MODE=false, JWT_SECRET is required and the process exits if missing.
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	devMode := os.Getenv("DEV_MODE") == "true"

	jwtSecret := os.Getenv("JWT_SECRET")
	if !devMode && jwtSecret == "" {
		fmt.Fprintln(os.Stderr, "FATAL: JWT_SECRET is required when DEV_MODE is not true")
		os.Exit(1)
	}
	if devMode && jwtSecret == "" {
		jwtSecret = "dev-secret"
	}

	expiry := 1 * time.Hour
	if v := os.Getenv("TOKEN_EXPIRY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: invalid TOKEN_EXPIRY %q: %v\n", v, err)
			os.Exit(1)
		}
		expiry = d
	}

	return Config{
		Port:        port,
		JWTSecret:   jwtSecret,
		TokenExpiry: expiry,
		DevMode:     devMode,
	}
}
