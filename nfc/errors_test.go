package nfc

import (
	"errors"
	"fmt"
	"testing"
)

func TestNFCError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *NFCError
		expected string
	}{
		{
			name: "with op and message",
			err: &NFCError{
				Code:    ErrCodeNotSupported,
				Op:      "Transceive",
				Message: "operation not supported",
			},
			expected: "Transceive: operation not supported",
		},
		{
			name: "with op, message, and cause",
			err: &NFCError{
				Code:    ErrCodeReadFailed,
				Op:      "ReadData",
				Message: "read failed",
				Cause:   errors.New("connection lost"),
			},
			expected: "ReadData: read failed: connection lost",
		},
		{
			name: "message only",
			err: &NFCError{
				Code:    ErrCodeNotSupported,
				Message: "not supported",
			},
			expected: "not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("NFCError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNFCError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &NFCError{
		Code:    ErrCodeReadFailed,
		Op:      "ReadData",
		Message: "read failed",
		Cause:   cause,
	}

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("NFCError.Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test without cause
	errNoCause := &NFCError{
		Code:    ErrCodeNotSupported,
		Message: "not supported",
	}
	if unwrapped := errNoCause.Unwrap(); unwrapped != nil {
		t.Errorf("NFCError.Unwrap() = %v, want nil", unwrapped)
	}
}

func TestNFCError_Is(t *testing.T) {
	err1 := &NFCError{Code: ErrCodeNotSupported, Message: "test"}
	err2 := &NFCError{Code: ErrCodeNotSupported, Message: "different message"}
	err3 := &NFCError{Code: ErrCodeReadFailed, Message: "test"}

	if !err1.Is(err2) {
		t.Error("NFCError.Is() should return true for same code")
	}

	if err1.Is(err3) {
		t.Error("NFCError.Is() should return false for different code")
	}

	if err1.Is(errors.New("not an NFCError")) {
		t.Error("NFCError.Is() should return false for non-NFCError")
	}
}

func TestNewNotSupportedError(t *testing.T) {
	err := NewNotSupportedError("Transceive")

	if err.Code != ErrCodeNotSupported {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeNotSupported)
	}
	if err.Op != "Transceive" {
		t.Errorf("Op = %q, want %q", err.Op, "Transceive")
	}
	if err.Message != "operation not supported" {
		t.Errorf("Message = %q, want %q", err.Message, "operation not supported")
	}
}

func TestNewAuthError(t *testing.T) {
	cause := errors.New("wrong key")
	err := NewAuthError("ReadData", "04A1B2C3", cause)

	if err.Code != ErrCodeAuthFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeAuthFailed)
	}
	if err.TagUID != "04A1B2C3" {
		t.Errorf("TagUID = %q, want %q", err.TagUID, "04A1B2C3")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestIsNotSupportedError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "NFCError with ErrCodeNotSupported",
			err:      NewNotSupportedError("Transceive"),
			expected: true,
		},
		{
			name:     "NFCError with different code",
			err:      &NFCError{Code: ErrCodeReadFailed, Message: "read failed"},
			expected: false,
		},
		{
			name:     "legacy string error - not supported",
			err:      fmt.Errorf("Transceive not supported for this tag"),
			expected: true,
		},
		{
			name:     "legacy string error - not directly supported",
			err:      fmt.Errorf("operation not directly supported"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("connection lost"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotSupportedError(tt.err); got != tt.expected {
				t.Errorf("IsNotSupportedError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsTagRemovedError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "NFCError with ErrCodeTagRemoved",
			err:      NewTagRemovedError("ReadData", nil),
			expected: true,
		},
		{
			name:     "legacy string - tag removed",
			err:      fmt.Errorf("tag removed during read"),
			expected: true,
		},
		{
			name:     "legacy string - Target was removed",
			err:      fmt.Errorf("Target was removed from field"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("connection lost"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTagRemovedError(tt.err); got != tt.expected {
				t.Errorf("IsTagRemovedError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "NFCError with ErrCodeAuthFailed",
			err:      NewAuthError("ReadData", "04A1B2C3", nil),
			expected: true,
		},
		{
			name:     "legacy string - authentication",
			err:      fmt.Errorf("authentication failed for sector 0"),
			expected: true,
		},
		{
			name:     "legacy string - auth",
			err:      fmt.Errorf("auth error"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("connection lost"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthError(tt.err); got != tt.expected {
				t.Errorf("IsAuthError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetErrorCode(t *testing.T) {
	nfcErr := &NFCError{Code: ErrCodeReadFailed}
	if code := GetErrorCode(nfcErr); code != ErrCodeReadFailed {
		t.Errorf("GetErrorCode() = %v, want %v", code, ErrCodeReadFailed)
	}

	regularErr := errors.New("regular error")
	if code := GetErrorCode(regularErr); code != 0 {
		t.Errorf("GetErrorCode() = %v, want 0", code)
	}
}

func TestErrorsAs(t *testing.T) {
	// Test that errors.As works with NFCError
	err := NewNotSupportedError("Transceive")
	wrappedErr := fmt.Errorf("operation failed: %w", err)

	var nfcErr *NFCError
	if !errors.As(wrappedErr, &nfcErr) {
		t.Error("errors.As should find NFCError in wrapped error")
	}
	if nfcErr.Code != ErrCodeNotSupported {
		t.Errorf("Code = %v, want %v", nfcErr.Code, ErrCodeNotSupported)
	}
}

func TestWrapError(t *testing.T) {
	cause := errors.New("underlying")
	err := WrapError(ErrCodeReadFailed, "ReadData", "read failed", cause)

	if err.Code != ErrCodeReadFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeReadFailed)
	}
	if err.Op != "ReadData" {
		t.Errorf("Op = %q, want %q", err.Op, "ReadData")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestErrorf(t *testing.T) {
	err := Errorf(ErrCodeWriteFailed, "WriteData", "failed to write %d bytes", 256)

	if err.Code != ErrCodeWriteFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeWriteFailed)
	}
	if err.Message != "failed to write 256 bytes" {
		t.Errorf("Message = %q, want %q", err.Message, "failed to write 256 bytes")
	}
}
