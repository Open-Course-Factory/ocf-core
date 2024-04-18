package middleware

import "github.com/gin-gonic/gin"

func CORS() gin.HandlerFunc {

	return func(ctx *gin.Context) {

		ctx.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		if ctx.Request.Method == "OPTIONS" {
			ctx.AbortWithStatus(204)
			return
		} else {
			ctx.Writer.Header().Set("Content-Type", "application/json")
		}
	}
}
