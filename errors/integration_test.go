package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jmgilman/go/errors"
	"github.com/stretchr/testify/require"
)

func TestErrorWorkflow_CreateWrapEnhance(t *testing.T) {
	// Simulate real-world error handling workflow

	// Layer 1: Database error
	dbErr := errors.New(errors.CodeDatabase, "connection failed")

	// Layer 2: Repository wraps with context
	repoErr := errors.Wrap(dbErr, errors.CodeDatabase, "failed to query users")
	repoErr = errors.WithContext(repoErr, "table", "users")
	repoErr = errors.WithContext(repoErr, "operation", "SELECT")

	// Layer 3: Service wraps with business context
	svcErr := errors.Wrap(repoErr, errors.CodeNotFound, "user not found")
	svcErr = errors.WithContext(svcErr, "user_id", "12345")

	// Layer 4: API handler adds request context
	apiErr := svcErr
	apiErr = errors.WithContext(apiErr, "endpoint", "/api/users/12345")
	apiErr = errors.WithContext(apiErr, "method", "GET")

	// Verify error chain
	require.Equal(t, errors.CodeNotFound, errors.GetCode(apiErr))
	require.Contains(t, apiErr.Error(), "user not found")
	require.Contains(t, apiErr.Error(), "failed to query users")

	// Verify context accumulation
	ctx := apiErr.Context()
	require.Equal(t, "/api/users/12345", ctx["endpoint"])
	require.Equal(t, "GET", ctx["method"])

	// Verify JSON serialization excludes internal layers
	resp := errors.ToJSON(apiErr)
	require.Equal(t, "NOT_FOUND", resp.Code)
	require.Equal(t, "user not found", resp.Message)

	// Internal error details not exposed
	jsonBytes, _ := json.Marshal(resp)
	jsonStr := string(jsonBytes)
	require.NotContains(t, jsonStr, "failed to query users")
	require.NotContains(t, jsonStr, "connection failed")
}

func TestErrorWorkflow_MultipleWrapping(t *testing.T) {
	// Create deep error chain
	err0 := stderrors.New("io: connection reset")
	err1 := errors.Wrap(err0, errors.CodeNetwork, "network error")
	err2 := errors.Wrap(err1, errors.CodeDatabase, "database connection failed")
	err3 := errors.Wrap(err2, errors.CodeInternal, "service unavailable")

	// Verify chain traversal
	require.True(t, stderrors.Is(err3, err0))
	require.True(t, errors.Is(err3, err1))
	require.True(t, errors.Is(err3, err2))

	// Verify classification preserved through chain
	require.True(t, errors.IsRetryable(err3)) // Network error is retryable
}

func TestErrorChain_StandardLibraryCompatibility(t *testing.T) {
	// Standard library sentinel
	var ErrNotFound = stderrors.New("not found")

	// Wrap with platform errors
	err1 := errors.Wrap(ErrNotFound, errors.CodeDatabase, "query failed")
	err2 := errors.Wrap(err1, errors.CodeNotFound, "resource not found")

	// Standard library errors.Is works
	require.True(t, stderrors.Is(err2, ErrNotFound))

	// Standard library errors.Unwrap works - manually traverse chain
	unwrapped1 := stderrors.Unwrap(err2)
	require.NotNil(t, unwrapped1)

	unwrapped2 := stderrors.Unwrap(unwrapped1)
	require.NotNil(t, unwrapped2)

	// The sentinel error is wrapped within the chain
	require.True(t, stderrors.Is(unwrapped2, ErrNotFound))
}

func TestErrorChain_TraversalDepth(t *testing.T) {
	// Create 10-level deep chain
	err := stderrors.New("root cause")
	for i := 0; i < 10; i++ {
		err = errors.Wrapf(err, errors.CodeInternal, "layer %d", i)
	}

	// Should be able to traverse entire chain
	depth := 0
	for e := err; e != nil; e = stderrors.Unwrap(e) {
		depth++
	}
	require.Equal(t, 11, depth) // 1 root + 10 wraps
}

func TestConcurrentErrorCreation(t *testing.T) {
	const goroutines = 100
	var wg sync.WaitGroup
	errs := make([]errors.PlatformError, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := errors.Newf(errors.CodeBuildFailed, "build %d failed", idx)
			err = errors.WithContext(err, "index", idx)
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify all errors created correctly
	for i, err := range errs {
		require.NotNil(t, err)
		require.Equal(t, errors.CodeBuildFailed, err.Code())
		require.Equal(t, i, err.Context()["index"])
	}
}

func TestConcurrentContextEnhancement(t *testing.T) {
	baseErr := errors.New(errors.CodeInternal, "base error")
	const goroutines = 50
	var wg sync.WaitGroup

	// Each goroutine adds different context
	enhanced := make([]errors.PlatformError, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := errors.WithContext(baseErr, fmt.Sprintf("key_%d", idx), idx)
			enhanced[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify each has its own context (immutability)
	for i, err := range enhanced {
		ctx := err.Context()
		require.Len(t, ctx, 1)
		require.Equal(t, i, ctx[fmt.Sprintf("key_%d", i)])
	}

	// Base error unchanged
	require.Nil(t, baseErr.Context())
}

func TestClassificationPropagation(t *testing.T) {
	// Retryable error
	retryable := errors.New(errors.CodeTimeout, "timeout")
	require.True(t, errors.IsRetryable(retryable))

	// Wrap multiple times
	wrapped1 := errors.Wrap(retryable, errors.CodeDatabase, "db timeout")
	wrapped2 := errors.Wrap(wrapped1, errors.CodeInternal, "internal timeout")

	// Classification preserved through chain
	require.True(t, errors.IsRetryable(wrapped1))
	require.True(t, errors.IsRetryable(wrapped2))

	// Override classification
	permanent := errors.WithClassification(wrapped2, errors.ClassificationPermanent)
	require.False(t, errors.IsRetryable(permanent))
}

func TestContextAccumulation(t *testing.T) {
	// Each layer adds its own context independently
	err1 := errors.New(errors.CodeBuildFailed, "build failed")
	err1 = errors.WithContext(err1, "layer1", "value1")

	err2 := errors.Wrap(err1, errors.CodeExecutionFailed, "execution failed")
	err2 = errors.WithContext(err2, "layer2", "value2")

	err3 := errors.Wrap(err2, errors.CodeInternal, "internal error")
	err3 = errors.WithContext(err3, "layer3", "value3")

	// Outermost error has its context
	ctx3 := err3.Context()
	require.Equal(t, "value3", ctx3["layer3"])

	// Inner errors have their own context (not merged)
	var err2Platform errors.PlatformError
	require.True(t, stderrors.As(err3.Unwrap(), &err2Platform))
	ctx2 := err2Platform.Context()
	require.Equal(t, "value2", ctx2["layer2"])
}
