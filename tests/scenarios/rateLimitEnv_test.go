package scenarios_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	scenarioMiddleware "soli/formations/src/scenarios/middleware"
)

// The limiter keeps its buckets in package state keyed by user, so every test
// here needs a user of its own or it inherits the previous test's window.
func limiterRouter(userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Next()
	})
	// PerUserRateLimit resolves its ceiling when the middleware is BUILT, not
	// per request — which is why each case sets the environment before this.
	r.POST("/x", scenarioMiddleware.PerUserRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func allowedBefore429(t *testing.T, r *gin.Engine, tries int) int {
	t.Helper()
	allowed := 0
	for i := 0; i < tries; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/x", nil)
		r.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			break
		}
		allowed++
	}
	return allowed
}

func TestScenarioRateLimit_DefaultsToTen(t *testing.T) {
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("SCENARIO_RATE_LIMIT_PER_MINUTE", "")

	assert.Equal(t, 10, allowedBefore429(t, limiterRouter("rl-default"), 20),
		"with no override the limit is the shipped default")
}

func TestScenarioRateLimit_RaisedInTestEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("SCENARIO_RATE_LIMIT_PER_MINUTE", "15")

	assert.Equal(t, 15, allowedBefore429(t, limiterRouter("rl-test-env"), 25),
		"a disposable environment may raise the ceiling")
}

func TestScenarioRateLimit_RaisedInDevelopmentEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("SCENARIO_RATE_LIMIT_PER_MINUTE", "13")

	assert.Equal(t, 13, allowedBefore429(t, limiterRouter("rl-dev-env"), 25))
}

// The point of the whole feature: production cannot be talked into a weaker
// limit, however the variable is set.
func TestScenarioRateLimit_IgnoresOverrideInProduction(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("SCENARIO_RATE_LIMIT_PER_MINUTE", "500")

	assert.Equal(t, 10, allowedBefore429(t, limiterRouter("rl-prod"), 20),
		"production ignores the override entirely")
}

// An unset ENVIRONMENT must read as production, not as "probably dev". A
// missing variable is exactly the shape a misconfigured deployment has, and it
// must never be the thing that unlocks a higher limit.
func TestScenarioRateLimit_IgnoresOverrideWhenEnvironmentUnset(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("SCENARIO_RATE_LIMIT_PER_MINUTE", "500")

	assert.Equal(t, 10, allowedBefore429(t, limiterRouter("rl-unset"), 20),
		"an unset environment is treated as production")
}

func TestScenarioRateLimit_IgnoresUnusableOverride(t *testing.T) {
	for _, bad := range []string{"nope", "0", "-4"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", "test")
			t.Setenv("SCENARIO_RATE_LIMIT_PER_MINUTE", bad)

			assert.Equal(t, 10, allowedBefore429(t, limiterRouter("rl-bad-"+bad), 20),
				"a value that is not a positive count falls back to the default")
		})
	}
}
