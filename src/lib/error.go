package lib

import (
	"fmt"
	"net/http"
)

type CustomError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *CustomError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func ErrorStruct(statusCode int, message string, err error) *CustomError {
	return &CustomError{
		StatusCode: statusCode,
		Message:    message,
		Err:        err,
	}
}

func BadRequest(message string, err error) *CustomError {
	return ErrorStruct(http.StatusBadRequest, message, err)
}

func Unauthorized(message string) *CustomError {
	return ErrorStruct(http.StatusUnauthorized, message, nil)
}

func Forbidden(message string) *CustomError {
	return ErrorStruct(http.StatusForbidden, message, nil)
}

func NotFound(message string) *CustomError {
	return ErrorStruct(http.StatusNotFound, message, nil)
}

func UnprocessableEntity(message string, err error) *CustomError {
	return ErrorStruct(http.StatusUnprocessableEntity, message, err)
}

func InternalServerError(message string, err error) *CustomError {
	return ErrorStruct(http.StatusInternalServerError, message, err)
}

func ServiceUnavailable(message string) *CustomError {
	return ErrorStruct(http.StatusServiceUnavailable, message, nil)
}

func HandleError(w http.ResponseWriter, err error) {
	if customErr, ok := err.(*CustomError); ok {
		Error(w, customErr.StatusCode, customErr.Message, customErr.Err)
	} else {
		customErr := InternalServerError("An unexpected error occurred", err)
		Error(w, customErr.StatusCode, customErr.Message, customErr.Err)
	}
}
