package errors

import "github.com/gin-gonic/gin"

type APIError struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func (apiError *APIError) Error() string {
	return apiError.ErrorMessage
}

func HandleError(code int, err error, ctx *gin.Context) bool {
	if err != nil {
		ctx.JSON(code, &APIError{
			ErrorCode:    code,
			ErrorMessage: err.Error(),
		})
		ctx.Abort()
		return true
	}
	return false
}

// Respond writes an APIError whose ErrorCode mirrors the HTTP status.
func Respond(ctx *gin.Context, code int, message string) {
	ctx.JSON(code, &APIError{ErrorCode: code, ErrorMessage: message})
}

// Abort is Respond for middleware: it also stops the handler chain.
func Abort(ctx *gin.Context, code int, message string) {
	ctx.AbortWithStatusJSON(code, &APIError{ErrorCode: code, ErrorMessage: message})
}
