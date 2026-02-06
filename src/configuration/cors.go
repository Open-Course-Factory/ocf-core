package config

import (
	"log"
	"net/url"
	"os"
	"strings"
)

var allowedOrigins []string

// InitAllowedOrigins builds the allowed origins list from environment variables.
// Must be called once at startup before any WebSocket or CORS usage.
func InitAllowedOrigins() []string {
	origins := []string{}
	environment := os.Getenv("ENVIRONMENT")

	if frontendURL := os.Getenv("FRONTEND_URL"); frontendURL != "" {
		origins = append(origins, frontendURL)
	}
	if adminURL := os.Getenv("ADMIN_FRONTEND_URL"); adminURL != "" {
		origins = append(origins, adminURL)
	}

	if environment == "development" || environment == "" || len(origins) == 0 {
		log.Println("Development mode: CORS allowing common localhost origins")
		origins = append(origins,
			"http://localhost:3000",
			"http://localhost:3001",
			"http://localhost:4000",
			"http://localhost:5173",
			"http://localhost:5174",
			"http://localhost:8080",
			"http://localhost:8081",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:4000",
			"http://127.0.0.1:5173",
			"http://127.0.0.1:8080",
		)
	}

	allowedOrigins = origins
	return origins
}

// GetAllowedOrigins returns the list of allowed origins.
func GetAllowedOrigins() []string {
	return allowedOrigins
}

// IsOriginAllowed checks if a given origin is in the allowed list.
// Used by WebSocket upgraders to validate the Origin header.
func IsOriginAllowed(origin string) bool {
	if len(allowedOrigins) == 0 {
		return false
	}

	for _, allowed := range allowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}

	// Also check by host for WebSocket origins that may differ in scheme (ws:// vs http://)
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	for _, allowed := range allowedOrigins {
		allowedURL, err := url.Parse(allowed)
		if err != nil {
			continue
		}
		if strings.EqualFold(originURL.Host, allowedURL.Host) {
			return true
		}
	}

	return false
}
