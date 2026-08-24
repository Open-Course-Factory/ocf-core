package middleware

import (
	"net/http"

	"soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EmailVerificationMiddleware interface {
	RequireVerifiedEmail() gin.HandlerFunc
}

type emailVerificationMiddleware struct {
	db *gorm.DB
}

func NewEmailVerificationMiddleware(db *gorm.DB) EmailVerificationMiddleware {
	return &emailVerificationMiddleware{db: db}
}

// RequireVerifiedEmail checks if the user has verified their email address
func (m *emailVerificationMiddleware) RequireVerifiedEmail() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")
		if userId == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "UNAUTHORIZED",
				"message": "User not authenticated",
			})
			return
		}

		// Ask the verification service rather than reading the Casdoor flag
		// here. That flag alone once refused 36 accounts that had genuinely
		// confirmed their address, because the service's PostgreSQL fallback
		// lived on the other side of this decision.
		verified, err := services.NewEmailVerificationService(m.db).IsEmailVerified(userId)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "UNAUTHORIZED",
				"message": "User not found",
			})
			return
		}

		if !verified {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "EMAIL_NOT_VERIFIED",
				"message":  "Please verify your email address to access this resource",
				"verified": false,
			})
			return
		}

		ctx.Next()
	}
}
