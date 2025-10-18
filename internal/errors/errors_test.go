package errors

import (
	"errors"
	"strings"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	appErr := &AppError{
		Code:    ExitValidationError,
		Message: "Invalid input",
		Cause:   nil,
	}

	got := appErr.Error()
	if !strings.Contains(got, "Invalid input") {
		t.Errorf("Error() should contain message")
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("Bad input", errors.New("cause"))

	if err.Code != ExitValidationError {
		t.Errorf("Expected exit code %d, got %d", ExitValidationError, err.Code)
	}
}

func TestNewHostnameConflictError(t *testing.T) {
	err := NewHostnameConflictError("testhost", "aa:bb:cc:dd:ee:ff")

	if err.Code != ExitHostnameConflict {
		t.Errorf("Expected exit code %d, got %d", ExitHostnameConflict, err.Code)
	}
	if !strings.Contains(err.Message, "testhost") {
		t.Error("Message should contain hostname")
	}
}

func TestAppError_ErrorWithHint(t *testing.T) {
	appErr := &AppError{
		Code:    ExitValidationError,
		Message: "Invalid",
	}

	got := appErr.ErrorWithHint()
	if !strings.Contains(got, "Action:") {
		t.Error("ErrorWithHint() should contain action hint")
	}
}
