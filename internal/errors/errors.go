// Package errors provides standardized error types for the API.
package errors

import (
	"fmt"
	"net/http"
)

// Code represents an API error code.
type Code string

const (
	CodeNotFound       Code = "NOT_FOUND"
	CodeInvalidID      Code = "INVALID_ID"
	CodeInvalidRequest Code = "INVALID_REQUEST"
	CodeInternal       Code = "INTERNAL_ERROR"
	CodeRateLimited    Code = "RATE_LIMITED"
)

// APIError represents a structured API error.
type APIError struct {
	Code       Code   `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *APIError) Error() string {
	return e.Message
}

// Common errors
var (
	ErrNotFound       = &APIError{Code: CodeNotFound, Message: "Resource not found", HTTPStatus: http.StatusNotFound}
	ErrInvalidID      = &APIError{Code: CodeInvalidID, Message: "Invalid ID format", HTTPStatus: http.StatusBadRequest}
	ErrInternal       = &APIError{Code: CodeInternal, Message: "Internal server error", HTTPStatus: http.StatusInternalServerError}
	ErrInvalidRequest = &APIError{Code: CodeInvalidRequest, Message: "Invalid request", HTTPStatus: http.StatusBadRequest}
	ErrRateLimited    = &APIError{Code: CodeRateLimited, Message: "Rate limit exceeded", HTTPStatus: http.StatusTooManyRequests}
)

// NotFound creates a not found error with a custom message.
func NotFound(resource string) *APIError {
	return &APIError{
		Code:       CodeNotFound,
		Message:    fmt.Sprintf("%s not found", resource),
		HTTPStatus: http.StatusNotFound,
	}
}

// InvalidID creates an invalid ID error with context.
func InvalidID(paramName string) *APIError {
	return &APIError{
		Code:       CodeInvalidID,
		Message:    fmt.Sprintf("Invalid %s: must be a positive integer", paramName),
		HTTPStatus: http.StatusBadRequest,
	}
}

// InvalidRequest creates a bad request error with a custom message.
func InvalidRequest(message string) *APIError {
	return &APIError{
		Code:       CodeInvalidRequest,
		Message:    message,
		HTTPStatus: http.StatusBadRequest,
	}
}

// Internal creates an internal error, optionally logging the real error.
func Internal(message string) *APIError {
	if message == "" {
		message = "Internal server error"
	}
	return &APIError{
		Code:       CodeInternal,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
	}
}
