package services

import (
	"errors"
	"net/http"
)

// ErasureErrorResponse maps an erasure failure to the HTTP status and message
// every erasure handler returns: self-service deletion, admin DELETE
// /users/:id and the organization owner's erase-now. It lives next to the
// sentinels so the three handlers cannot drift apart. Pre-flight refusals
// carry the service message so the caller knows what to fix first; anything
// else is retryable.
func ErasureErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, ErrUserNotFound):
		return http.StatusNotFound, "User not found"
	case errors.Is(err, ErrOwnsOrganizations), errors.Is(err, ErrOwnsGroups), errors.Is(err, ErrStillActiveElsewhere):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "User erasure failed"
	}
}
