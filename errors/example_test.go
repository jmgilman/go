package errors_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jmgilman/go/errors"
)

func ExampleNew() {
	err := errors.New(errors.CodeNotFound, "user not found")
	fmt.Println(err.Error())
	// Output: [NOT_FOUND] user not found
}

func ExampleNewf() {
	userID := "12345"
	err := errors.Newf(errors.CodeNotFound, "user %s not found", userID)
	fmt.Println(err.Error())
	// Output: [NOT_FOUND] user 12345 not found
}

func ExampleWrap() {
	// Simulate database error
	dbErr := fmt.Errorf("connection refused")

	// Wrap with platform error
	err := errors.Wrap(dbErr, errors.CodeDatabase, "failed to connect to database")

	fmt.Println(errors.GetCode(err))
	// Output: DATABASE_ERROR
}

func ExampleWithContext() {
	err := errors.New(errors.CodeBuildFailed, "build failed")
	err = errors.WithContext(err, "project", "api")
	err = errors.WithContext(err, "phase", "test")

	ctx := err.Context()
	fmt.Printf("Project: %s, Phase: %s\n", ctx["project"], ctx["phase"])
	// Output: Project: api, Phase: test
}

func ExampleWithContextMap() {
	err := errors.New(errors.CodeExecutionFailed, "execution failed")
	err = errors.WithContextMap(err, map[string]interface{}{
		"command":   "earthly",
		"target":    "+test",
		"exit_code": 1,
	})

	ctx := err.Context()
	fmt.Printf("Command: %s, Exit: %d\n", ctx["command"], ctx["exit_code"])
	// Output: Command: earthly, Exit: 1
}

func ExampleIsRetryable() {
	// Retryable error
	timeoutErr := errors.New(errors.CodeTimeout, "request timeout")
	fmt.Println("Timeout retryable:", errors.IsRetryable(timeoutErr))

	// Permanent error
	notFoundErr := errors.New(errors.CodeNotFound, "user not found")
	fmt.Println("NotFound retryable:", errors.IsRetryable(notFoundErr))

	// Output:
	// Timeout retryable: true
	// NotFound retryable: false
}

func ExampleIsRetryable_retryLoop() {
	operation := func() error {
		// Simulate operation that might fail
		return errors.New(errors.CodeNetwork, "connection refused")
	}

	const maxRetries = 3
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		err = operation()
		if err == nil {
			fmt.Println("Success")
			return
		}

		if !errors.IsRetryable(err) {
			fmt.Println("Permanent error, not retrying")
			return
		}

		fmt.Printf("Attempt %d failed, retrying...\n", attempt+1)
		time.Sleep(100 * time.Millisecond) // Backoff
	}

	fmt.Println("Max retries exceeded")
	// Output:
	// Attempt 1 failed, retrying...
	// Attempt 2 failed, retrying...
	// Attempt 3 failed, retrying...
	// Max retries exceeded
}

func ExampleToJSON() {
	err := errors.New(errors.CodeInvalidInput, "validation failed")
	err = errors.WithContext(err, "field", "email")
	err = errors.WithContext(err, "reason", "invalid format")

	response := errors.ToJSON(err)
	jsonBytes, _ := json.MarshalIndent(response, "", "  ")
	fmt.Println(string(jsonBytes))

	// Output:
	// {
	//   "code": "INVALID_INPUT",
	//   "message": "validation failed",
	//   "classification": "PERMANENT",
	//   "context": {
	//     "field": "email",
	//     "reason": "invalid format"
	//   }
	// }
}

func ExampleToJSON_httpHandler() {
	// Example HTTP error handler
	handleError := func(w http.ResponseWriter, err error) {
		response := errors.ToJSON(err)

		w.Header().Set("Content-Type", "application/json")

		// Map error code to HTTP status
		statusCode := http.StatusInternalServerError
		switch errors.GetCode(err) {
		case errors.CodeNotFound:
			statusCode = http.StatusNotFound
		case errors.CodeUnauthorized:
			statusCode = http.StatusUnauthorized
		case errors.CodeInvalidInput:
			statusCode = http.StatusBadRequest
		}

		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(response)
	}

	// Simulate error
	err := errors.New(errors.CodeNotFound, "resource not found")

	// Would write HTTP response
	_ = handleError
	fmt.Println(errors.GetCode(err))
	// Output: NOT_FOUND
}

func ExampleWithClassification() {
	// Override default classification
	err := errors.New(errors.CodeDatabase, "database error")
	fmt.Println("Default:", errors.IsRetryable(err))

	// Mark as permanent for this specific case
	err = errors.WithClassification(err, errors.ClassificationPermanent)
	fmt.Println("Overridden:", errors.IsRetryable(err))

	// Output:
	// Default: true
	// Overridden: false
}

func ExampleGetCode() {
	err := errors.New(errors.CodeNotFound, "user not found")

	if errors.GetCode(err) == errors.CodeNotFound {
		fmt.Println("Handle not found case")
	}

	// Output: Handle not found case
}

func ExampleIs() {
	// Create sentinel error
	var ErrUserNotFound = errors.New(errors.CodeNotFound, "user not found")

	// Wrap the sentinel
	err := errors.Wrap(ErrUserNotFound, errors.CodeDatabase, "query failed")

	// Check if error chain contains sentinel
	if errors.Is(err, ErrUserNotFound) {
		fmt.Println("User not found in chain")
	}

	// Output: User not found in chain
}

func ExampleAs() {
	err := errors.New(errors.CodeTimeout, "request timeout")
	wrapped := errors.Wrap(err, errors.CodeInternal, "internal error")

	// Extract PlatformError from chain
	var platformErr errors.PlatformError
	if errors.As(wrapped, &platformErr) {
		fmt.Println("Code:", platformErr.Code())
		fmt.Println("Retryable:", platformErr.Classification().IsRetryable())
	}

	// Output:
	// Code: INTERNAL_ERROR
	// Retryable: true
}

// Example_workflow shows a complete error handling workflow across multiple layers.
func Example_workflow() {
	// Layer 1: Database layer
	dbErr := fmt.Errorf("connection timeout")

	// Layer 2: Repository layer wraps and adds context
	repoErr := errors.Wrap(dbErr, errors.CodeDatabase, "failed to query users")
	repoErr = errors.WithContext(repoErr, "table", "users")

	// Layer 3: Service layer wraps with business error
	svcErr := errors.Wrap(repoErr, errors.CodeNotFound, "user not found")
	svcErr = errors.WithContext(svcErr, "user_id", "12345")

	// Check if retryable
	fmt.Println("Retryable:", errors.IsRetryable(svcErr))

	// Get error code
	fmt.Println("Code:", errors.GetCode(svcErr))

	// Serialize for API
	response := errors.ToJSON(svcErr)
	fmt.Println("API Code:", response.Code)
	fmt.Println("API Message:", response.Message)

	// Output:
	// Retryable: true
	// Code: NOT_FOUND
	// API Code: NOT_FOUND
	// API Message: user not found
}
