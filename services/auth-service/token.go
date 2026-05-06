// ECR push test - 2026-04-25
package main

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateToken creates a signed JWT with the given user ID.
// The token includes sub (user ID), iat (issued at), and exp (expiration) claims.
func GenerateToken(userID string, secret string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(expiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
