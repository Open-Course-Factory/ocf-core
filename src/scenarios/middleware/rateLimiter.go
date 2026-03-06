package middleware

import (
	"net/http"
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
	buckets sync.Map // map[string]*userBucket
)

const (
	maxRequests = 10
	windowSize  = time.Minute
)

// PerUserRateLimit returns a Gin middleware that limits requests to
// maxRequests per windowSize per authenticated user.
func PerUserRateLimit() gin.HandlerFunc {
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
	}
}
