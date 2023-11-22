package errors

type APIError struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func (apiError *APIError) Error() string {
	return apiError.ErrorMessage
}
