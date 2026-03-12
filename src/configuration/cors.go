package config

import (
	"log"
	"net/url"
	"os"
	"strings"
)

var allowedOrigins []string
var allowLocalhost bool

// InitAllowedOrigins builds the allowed origins list from environment variables.
// Must be called once at startup before any WebSocket or CORS usage.
func InitAllowedOrigins() []string {
	origins := []string{}
	environment := os.Getenv("ENVIRONMENT")
	allowLocalhost = false

	if frontendURL := os.Getenv("FRONTEND_URL"); frontendURL != "" {
		origins = append(origins, frontendURL)
	}
	if adminURL := os.Getenv("ADMIN_FRONTEND_URL"); adminURL != "" {
		origins = append(origins, adminURL)
	}

	if environment == "development" || environment == "" || len(origins) == 0 {
		log.Println("Development mode: CORS allowing all localhost origins (any port)")
		allowLocalhost = true
	}

	allowedOrigins = origins
	return origins
}

// GetAllowedOrigins returns the list of allowed origins.
func GetAllowedOrigins() []string {
	return allowedOrigins
}

// IsOriginAllowed checks if a given origin is in the allowed list.
// Used by WebSocket upgraders and CORS AllowOriginFunc to validate the Origin header.
func IsOriginAllowed(origin string) bool {
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// In development mode, allow any localhost/127.0.0.1 origin regardless of port
	if allowLocalhost {
		hostname := originURL.Hostname()
		if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
			return true
		}
	}

	for _, allowed := range allowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}

	// Also check by host for WebSocket origins that may differ in scheme (ws:// vs http://)
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
