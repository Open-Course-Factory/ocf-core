package utils

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Logger provides structured logging with environment-aware debug levels
type Logger struct {
	debugEnabled bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// getLogger lazily initializes and returns the logger
// This ensures ENVIRONMENT variable is read after .env is loaded
func getLogger() *Logger {
	once.Do(func() {
		env := strings.ToLower(os.Getenv("ENVIRONMENT"))
		debugEnabled := env == "development" || env == "dev" || env == "test"
		defaultLogger = &Logger{debugEnabled: debugEnabled}

		// Log the logger initialization for debugging
		if debugEnabled {
			log.Printf("[DEBUG] Logger initialized in %s mode (debug enabled)", env)
		}
	})
	return defaultLogger
}

// Debug logs debug messages (only in development)
func Debug(format string, args ...any) {
	if getLogger().debugEnabled {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs informational messages (always)
func Info(format string, args ...any) {
	log.Printf("[INFO] "+format, args...)
}

// Warn logs warning messages (always)
func Warn(format string, args ...any) {
	log.Printf("[WARN] "+format, args...)
}

// Error logs error messages (always)
func Error(format string, args ...any) {
	log.Printf("[ERROR] "+format, args...)
}

// Printf provides backward compatibility with fmt.Printf
// Deprecated: Use Debug, Info, Warn, or Error instead
func Printf(format string, args ...any) {
	if defaultLogger.debugEnabled {
		fmt.Printf(format, args...)
	}
}

// IsDebugEnabled returns whether debug logging is enabled
func IsDebugEnabled() bool {
	return getLogger().debugEnabled
}
