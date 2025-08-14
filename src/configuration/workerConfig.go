// src/configuration/workerConfig.go
package config

import (
	"os"
	"strconv"
	"time"
)

type WorkerConfig struct {
	URL          string        `json:"url"`
	Timeout      time.Duration `json:"timeout"`
	RetryCount   int           `json:"retry_count"`
	PollInterval time.Duration `json:"poll_interval"`
}

func LoadWorkerConfig() *WorkerConfig {
	config := &WorkerConfig{
		URL:          getEnv("OCF_WORKER_URL", "http://localhost:8081"),
		Timeout:      getDurationEnv("OCF_WORKER_TIMEOUT", 300) * time.Second,
		RetryCount:   getIntEnv("OCF_WORKER_RETRY_COUNT", 3),
		PollInterval: getDurationEnv("OCF_WORKER_POLL_INTERVAL", 5) * time.Second,
	}
	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue int) time.Duration {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return time.Duration(intValue)
		}
	}
	return time.Duration(defaultValue)
}
