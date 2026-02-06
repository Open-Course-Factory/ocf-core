package middleware

import (
	"net/http"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
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

		// Get user from Casdoor
		user, err := casdoorsdk.GetUserByUserId(userId)
		if err != nil || user == nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "UNAUTHORIZED",
				"message": "User not found",
			})
			return
		}

		// Check verification status
		emailVerified := false
		if user.Properties != nil {
			emailVerified = user.Properties["email_verified"] == "true"
		}

		if !emailVerified {
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
