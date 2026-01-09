package nfc

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCode represents a specific type of NFC error for programmatic handling.
type ErrorCode int

const (
	// Tag operation errors (100-199)
	ErrCodeNotSupported ErrorCode = iota + 100
	ErrCodeTagRemoved
	ErrCodeAuthFailed
	ErrCodeReadFailed
	ErrCodeWriteFailed
	ErrCodeTransceiveFailed
	ErrCodeTagNotConnected
	ErrCodeReadOnly
	ErrCodeCapacityExceeded
	ErrCodeInvalidData
)

// NFCError provides structured error information for programmatic handling.
type NFCError struct {
	Code    ErrorCode
	Op      string // Operation that failed (e.g., "ReadData", "Transceive")
	TagUID  string // Optional: UID of tag involved
	Message string // Human-readable message
	Cause   error  // Underlying error
}

func (e *NFCError) Error() string {
	var sb strings.Builder
	if e.Op != "" {
		sb.WriteString(e.Op)
		sb.WriteString(": ")
	}
	sb.WriteString(e.Message)
	if e.Cause != nil {
		sb.WriteString(": ")
		sb.WriteString(e.Cause.Error())
	}
	return sb.String()
}

func (e *NFCError) Unwrap() error {
	return e.Cause
}

func (e *NFCError) Is(target error) bool {
	if t, ok := target.(*NFCError); ok {
		return e.Code == t.Code
	}
	return false
}

// NewNotSupportedError creates an error for unsupported operations.
func NewNotSupportedError(op string) *NFCError {
	return &NFCError{
		Code:    ErrCodeNotSupported,
		Op:      op,
		Message: "operation not supported",
	}
}

// NewTagRemovedError creates an error for when a tag is removed mid-operation.
func NewTagRemovedError(op string, cause error) *NFCError {
	return &NFCError{
		Code:    ErrCodeTagRemoved,
		Op:      op,
		Message: "tag removed during operation",
		Cause:   cause,
	}
}

// NewAuthError creates an error for authentication failures.
func NewAuthError(op, tagUID string, cause error) *NFCError {
	return &NFCError{
		Code:    ErrCodeAuthFailed,
		Op:      op,
		TagUID:  tagUID,
		Message: "authentication failed",
		Cause:   cause,
	}
}

// NewReadError creates an error for read failures.
func NewReadError(op string, cause error) *NFCError {
	return &NFCError{
		Code:    ErrCodeReadFailed,
		Op:      op,
		Message: "read failed",
		Cause:   cause,
	}
}

// NewWriteError creates an error for write failures.
func NewWriteError(op string, cause error) *NFCError {
	return &NFCError{
		Code:    ErrCodeWriteFailed,
		Op:      op,
		Message: "write failed",
		Cause:   cause,
	}
}

// NewTransceiveError creates an error for transceive failures.
func NewTransceiveError(op string, cause error) *NFCError {
	return &NFCError{
		Code:    ErrCodeTransceiveFailed,
		Op:      op,
		Message: "transceive failed",
		Cause:   cause,
	}
}

// IsNotSupportedError checks if an error indicates an unsupported operation.
func IsNotSupportedError(err error) bool {
	if err == nil {
		return false
	}
	var nfcErr *NFCError
	if errors.As(err, &nfcErr) {
		return nfcErr.Code == ErrCodeNotSupported
	}
	// Fallback to string matching for backward compatibility
	errStr := err.Error()
	return strings.Contains(errStr, "not supported") ||
		strings.Contains(errStr, "not directly supported") ||
		strings.Contains(errStr, "operation not supported")
}

// IsTagRemovedError checks if an error indicates the tag was removed.
func IsTagRemovedError(err error) bool {
	if err == nil {
		return false
	}
	var nfcErr *NFCError
	if errors.As(err, &nfcErr) {
		return nfcErr.Code == ErrCodeTagRemoved
	}
	// Fallback to string matching
	errStr := err.Error()
	return strings.Contains(errStr, "tag removed") ||
		strings.Contains(errStr, "tag lost") ||
		strings.Contains(errStr, "Target was removed")
}

// IsAuthError checks if an error indicates authentication failure.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	var nfcErr *NFCError
	if errors.As(err, &nfcErr) {
		return nfcErr.Code == ErrCodeAuthFailed
	}
	// Fallback to string matching
	errStr := err.Error()
	return strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "auth")
}

// GetErrorCode extracts the ErrorCode from an error if it's an NFCError.
// Returns 0 if the error is not an NFCError.
func GetErrorCode(err error) ErrorCode {
	var nfcErr *NFCError
	if errors.As(err, &nfcErr) {
		return nfcErr.Code
	}
	return 0
}

// WrapError wraps an existing error with NFC context.
func WrapError(code ErrorCode, op, message string, cause error) *NFCError {
	return &NFCError{
		Code:    code,
		Op:      op,
		Message: message,
		Cause:   cause,
	}
}

// Errorf creates an NFCError with a formatted message.
func Errorf(code ErrorCode, op, format string, args ...interface{}) *NFCError {
	return &NFCError{
		Code:    code,
		Op:      op,
		Message: fmt.Sprintf(format, args...),
	}
}
