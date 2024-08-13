package accessController

import (
	"github.com/gin-gonic/gin"
)

type AccessController interface {
	AddEntityAccesses(ctx *gin.Context)
	DeleteEntityAccesses(ctx *gin.Context)
}

type accessController struct {
}

func NewAccessController() AccessController {
	return &accessController{}
}
