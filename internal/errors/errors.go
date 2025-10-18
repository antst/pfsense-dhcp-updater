// Package errors provides application-specific error types with exit codes and actionable messages.
package errors

import "fmt"

// Exit codes for the application
const (
	ExitSuccess          = 0 // Success or noop
	ExitValidationError  = 1 // Input validation failed
	ExitHostnameConflict = 2 // Hostname already in use
	ExitAPIError         = 3 // pfSense API error
	ExitConfigError      = 4 // Configuration error
	ExitNetworkTimeout   = 5 // Network timeout
)

// AppError represents an application error with context and actionable hints.
type AppError struct {
	Code    int    // Exit code
	Message string // User-facing error message
	Cause   error  // Underlying error (may be nil)
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[ERROR] %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("[ERROR] %s", e.Message)
}

// ErrorWithHint returns the full error message with an actionable hint.
func (e *AppError) ErrorWithHint() string {
	hint := e.actionHint()
	if e.Cause != nil {
		return fmt.Sprintf("[ERROR] %s: %v\nAction: %s", e.Message, e.Cause, hint)
	}
	return fmt.Sprintf("[ERROR] %s\nAction: %s", e.Message, hint)
}

// actionHint provides an actionable hint based on the error code.
func (e *AppError) actionHint() string {
	switch e.Code {
	case ExitValidationError:
		return "Check input format (MAC: XX:XX:XX:XX:XX:XX, hostname: RFC 1123, VLAN: 0-4094)"
	case ExitHostnameConflict:
		return "Choose a different hostname or remove the existing mapping in pfSense"
	case ExitAPIError:
		return "Check pfSense API accessibility, credentials, and DHCP server status"
	case ExitConfigError:
		return "Verify config file format, required fields, and environment variables"
	case ExitNetworkTimeout:
		return "Check network connectivity to pfSense and consider increasing timeout"
	default:
		return "See logs for details or use --verbose for more information"
	}
}

// Unwrap returns the underlying error for error chain unwrapping.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// NewValidationError creates a validation error (exit code 1).
func NewValidationError(message string, cause error) *AppError {
	return &AppError{
		Code:    ExitValidationError,
		Message: message,
		Cause:   cause,
	}
}

// NewHostnameConflictError creates a hostname conflict error (exit code 2).
func NewHostnameConflictError(hostname, existingMAC string) *AppError {
	return &AppError{
		Code:    ExitHostnameConflict,
		Message: fmt.Sprintf("Hostname '%s' is already mapped to MAC address %s", hostname, existingMAC),
		Cause:   nil,
	}
}

// NewAPIError creates an API error (exit code 3).
func NewAPIError(message string, cause error) *AppError {
	return &AppError{
		Code:    ExitAPIError,
		Message: message,
		Cause:   cause,
	}
}

// NewConfigError creates a configuration error (exit code 4).
func NewConfigError(message string, cause error) *AppError {
	return &AppError{
		Code:    ExitConfigError,
		Message: message,
		Cause:   cause,
	}
}

// NewNetworkTimeoutError creates a network timeout error (exit code 5).
func NewNetworkTimeoutError(message string, cause error) *AppError {
	return &AppError{
		Code:    ExitNetworkTimeout,
		Message: message,
		Cause:   cause,
	}
}
