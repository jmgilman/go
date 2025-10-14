package errors

import (
	"encoding/json"
)

// ErrorResponse represents the JSON structure for error responses in API endpoints.
// It provides a flat, serializable representation of errors without exposing
// internal error chains or sensitive information.
//
// The wrapped error chain is intentionally excluded to prevent information leakage
// while still providing useful debugging context through the Code, Message, and Context fields.
type ErrorResponse struct {
	// Code is the error code identifying the type of error.
	Code string `json:"code"`

	// Message is the human-readable error message.
	Message string `json:"message"`

	// Classification indicates whether the error is retryable or permanent.
	Classification string `json:"classification"`

	// Context contains optional metadata about the error.
	// Omitted from JSON if empty.
	Context map[string]interface{} `json:"context,omitempty"`
}

// ToJSON converts any error to an ErrorResponse suitable for JSON serialization.
// Returns nil if err is nil.
//
// For PlatformError instances, extracts code, message, classification, and context.
// For standard errors, uses CodeUnknown, ClassificationPermanent, and the error message.
//
// The wrapped error chain is intentionally excluded to prevent information leakage.
// Security consideration: Error chains may contain internal implementation details,
// stack traces, database queries, file paths, or other sensitive information.
//
// Example:
//
//	func handleError(w http.ResponseWriter, err error) {
//	    response := errors.ToJSON(err)
//	    if response == nil {
//	        return // No error
//	    }
//	    w.Header().Set("Content-Type", "application/json")
//	    statusCode := getHTTPStatus(response.Code)
//	    w.WriteHeader(statusCode)
//	    json.NewEncoder(w).Encode(response)
//	}
func ToJSON(err error) *ErrorResponse {
	if err == nil {
		return nil
	}

	// Extract values using helper functions
	code := GetCode(err)
	classification := GetClassification(err)

	// Get message and context if PlatformError
	message := err.Error()
	var context map[string]interface{}

	var platformErr PlatformError
	if As(err, &platformErr) {
		message = platformErr.Message()
		context = platformErr.Context()
	}

	return &ErrorResponse{
		Code:           string(code),
		Message:        message,
		Classification: string(classification),
		Context:        context,
	}
}

// MarshalJSON implements json.Marshaler for platformError.
// This allows PlatformError instances to be marshaled directly using json.Marshal
// without needing to call ToJSON explicitly.
//
// Example:
//
//	err := errors.New(errors.CodeNotFound, "user not found")
//	jsonBytes, _ := json.Marshal(err)
//	// Output: {"code":"NOT_FOUND","message":"user not found","classification":"PERMANENT"}
//
//	// Or in a struct:
//	type Response struct {
//	    Success bool          `json:"success"`
//	    Error   PlatformError `json:"error,omitempty"`
//	}
func (e *platformError) MarshalJSON() ([]byte, error) {
	response := &ErrorResponse{
		Code:           string(e.code),
		Message:        e.message,
		Classification: string(e.classification),
		Context:        e.context,
	}
	data, err := json.Marshal(response)
	if err != nil {
		// Wrap the error to satisfy wrapcheck linter
		// This should rarely happen as ErrorResponse fields are simple types
		return nil, &platformError{
			code:           CodeInternal,
			classification: ClassificationPermanent,
			message:        "failed to marshal error response",
			cause:          err,
		}
	}
	return data, nil
}
