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
//
// FRONTEND_URL and ADMIN_FRONTEND_URL are expected to be the canonical
// single URL of the corresponding frontend (used elsewhere to build email
// links, etc.). EXTRA_FRONTEND_ORIGINS is the place to add additional
// CORS-allowed origins without overloading FRONTEND_URL — useful during
// domain migrations when two frontend hosts must coexist.
//
// All three are parsed as comma-separated for forward compatibility, but
// only EXTRA_FRONTEND_ORIGINS is intended to carry multiple values.
func InitAllowedOrigins() []string {
	origins := []string{}
	environment := os.Getenv("ENVIRONMENT")

	origins = append(origins, parseOriginList(os.Getenv("FRONTEND_URL"))...)
	origins = append(origins, parseOriginList(os.Getenv("ADMIN_FRONTEND_URL"))...)
	origins = append(origins, parseOriginList(os.Getenv("EXTRA_FRONTEND_ORIGINS"))...)

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

// parseOriginList splits a comma-separated env-var value into a clean list
// of origins. Whitespace is trimmed; empty entries are dropped.
func parseOriginList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
