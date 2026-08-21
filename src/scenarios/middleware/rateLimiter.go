package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// userBucket tracks requests for a single user using a sliding window.
type userBucket struct {
	mu         sync.Mutex
	timestamps []time.Time
}

var (
	buckets     sync.Map // map[string]*userBucket
	lastCleanup time.Time
	cleanupMu   sync.Mutex
)

const (
	defaultMaxRequests = 10
	windowSize         = time.Minute
	cleanupInterval    = 5 * time.Minute
	staleThreshold     = 2 * time.Minute

	// Raises the ceiling, and ONLY outside production. See resolveMaxRequests.
	maxRequestsEnvVar = "SCENARIO_RATE_LIMIT_PER_MINUTE"
)

// nonProductionEnvironments may raise the limit. This is an allow-list rather
// than a "not production" test on purpose: an unset or unrecognised
// ENVIRONMENT is exactly the shape a misconfigured deployment has, and it must
// never be the thing that unlocks a weaker limit.
var nonProductionEnvironments = map[string]bool{
	"development": true,
	"dev":         true,
	"test":        true,
}

// resolveMaxRequests returns the per-user ceiling for a window.
//
// The limit is a policy — it keeps one learner from hammering the container
// fleet — so production always gets the shipped default and there is no way to
// raise it there. A disposable environment protects nothing and pays a real
// cost for the limiter: playing a scenario end to end spends most of its wall
// clock waiting out a window rather than testing anything, because every step
// checks twice (refuse before the work, accept after).
//
// It is read when the middleware is BUILT, not per request, so it lands after
// the .env file has been loaded and cannot be changed under a running server.
func resolveMaxRequests() int {
	if !nonProductionEnvironments[strings.ToLower(os.Getenv("ENVIRONMENT"))] {
		return defaultMaxRequests
	}

	raw := os.Getenv(maxRequestsEnvVar)
	if raw == "" {
		return defaultMaxRequests
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		slog.Warn("ignoring scenario rate limit override: not a positive whole number",
			"var", maxRequestsEnvVar, "value", raw, "using", defaultMaxRequests)
		return defaultMaxRequests
	}

	slog.Info("scenario rate limit raised for a non-production environment",
		"var", maxRequestsEnvVar, "limit", n, "default", defaultMaxRequests)
	return n
}

// PerUserRateLimit returns a Gin middleware that limits requests to
// maxRequests per windowSize per authenticated user.
func PerUserRateLimit() gin.HandlerFunc {
	maxRequests := resolveMaxRequests()

	return func(ctx *gin.Context) {
		userID := ctx.GetString("userId")
		if userID == "" {
			ctx.Next()
			return
		}

		now := time.Now()

		val, _ := buckets.LoadOrStore(userID, &userBucket{})
		bucket := val.(*userBucket)

		bucket.mu.Lock()

		// Prune expired timestamps outside the window
		cutoff := now.Add(-windowSize)
		pruned := make([]time.Time, 0, len(bucket.timestamps))
		for _, ts := range bucket.timestamps {
			if ts.After(cutoff) {
				pruned = append(pruned, ts)
			}
		}

		if len(pruned) >= maxRequests {
			bucket.timestamps = pruned
			bucket.mu.Unlock()
			ctx.JSON(http.StatusTooManyRequests, &errors.APIError{
				ErrorCode:    http.StatusTooManyRequests,
				ErrorMessage: "Rate limit exceeded. Try again later.",
			})
			ctx.Abort()
			return
		}

		bucket.timestamps = append(pruned, now)
		bucket.mu.Unlock()
		ctx.Next()

		// Periodically evict stale entries to prevent memory leak
		if time.Since(lastCleanup) > cleanupInterval {
			cleanupMu.Lock()
			if time.Since(lastCleanup) > cleanupInterval { // double-check after lock
				lastCleanup = time.Now()
				buckets.Range(func(key, value interface{}) bool {
					b := value.(*userBucket)
					b.mu.Lock()
					if len(b.timestamps) == 0 || time.Since(b.timestamps[len(b.timestamps)-1]) > staleThreshold {
						buckets.Delete(key)
					}
					b.mu.Unlock()
					return true
				})
			}
			cleanupMu.Unlock()
		}
	}
}
