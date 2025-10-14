package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"testing"

	"github.com/jmgilman/go/errors"
)

// BenchmarkNew measures error creation performance.
// Target: <10μs per operation.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.New(errors.CodeNotFound, "resource not found")
	}
}

func BenchmarkNewf(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.Newf(errors.CodeInvalidInput, "invalid value: %d", 42)
	}
}

// BenchmarkWrap measures error wrapping performance.
// Target: <5μs per operation.
func BenchmarkWrap(b *testing.B) {
	baseErr := stderrors.New("base error")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.Wrap(baseErr, errors.CodeDatabase, "database error")
	}
}

func BenchmarkWrapf(b *testing.B) {
	baseErr := stderrors.New("base error")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.Wrapf(baseErr, errors.CodeDatabase, "query failed: %s", "timeout")
	}
}

func BenchmarkWrapWithContext(b *testing.B) {
	baseErr := stderrors.New("base error")
	ctx := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.WrapWithContext(baseErr, errors.CodeInternal, "internal error", ctx)
	}
}

// BenchmarkWithContext measures context enhancement performance.
// Target: <2μs per operation.
func BenchmarkWithContext(b *testing.B) {
	baseErr := errors.New(errors.CodeInternal, "internal error")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.WithContext(baseErr, "key", "value")
	}
}

func BenchmarkWithContext_Chaining(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := errors.New(errors.CodeBuildFailed, "build failed")
		err = errors.WithContext(err, "project", "api")
		err = errors.WithContext(err, "phase", "test")
		err = errors.WithContext(err, "exit_code", 1)
		_ = err
	}
}

func BenchmarkWithContextMap(b *testing.B) {
	baseErr := errors.New(errors.CodeInternal, "internal error")
	ctx := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.WithContextMap(baseErr, ctx)
	}
}

func BenchmarkWithContextMap_Large(b *testing.B) {
	baseErr := errors.New(errors.CodeInternal, "internal error")

	// Large context map (20 fields)
	ctx := make(map[string]interface{})
	for i := 0; i < 20; i++ {
		ctx[string(rune('a'+i))] = i
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.WithContextMap(baseErr, ctx)
	}
}

func BenchmarkWithClassification(b *testing.B) {
	baseErr := errors.New(errors.CodeNotFound, "not found")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.WithClassification(baseErr, errors.ClassificationRetryable)
	}
}

func BenchmarkGetCode(b *testing.B) {
	err := errors.New(errors.CodeNotFound, "not found")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.GetCode(err)
	}
}

func BenchmarkGetCode_DeepChain(b *testing.B) {
	// Create 10-level deep chain
	err := stderrors.New("root")
	for i := 0; i < 10; i++ {
		err = errors.Wrap(err, errors.CodeInternal, "wrapped")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.GetCode(err)
	}
}

func BenchmarkGetClassification(b *testing.B) {
	err := errors.New(errors.CodeTimeout, "timeout")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.GetClassification(err)
	}
}

func BenchmarkIsRetryable(b *testing.B) {
	err := errors.New(errors.CodeTimeout, "timeout")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.IsRetryable(err)
	}
}

func BenchmarkIsRetryable_DeepChain(b *testing.B) {
	var err error = errors.New(errors.CodeNetwork, "network error")
	for i := 0; i < 10; i++ {
		err = errors.Wrap(err, errors.CodeDatabase, "wrapped")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.IsRetryable(err)
	}
}

func BenchmarkToJSON(b *testing.B) {
	err := errors.New(errors.CodeNotFound, "resource not found")
	err = errors.WithContextMap(err, map[string]interface{}{
		"id":   "12345",
		"type": "user",
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.ToJSON(err)
	}
}

func BenchmarkMarshalJSON(b *testing.B) {
	err := errors.New(errors.CodeNotFound, "resource not found")
	err = errors.WithContextMap(err, map[string]interface{}{
		"id":   "12345",
		"type": "user",
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(err)
	}
}

func BenchmarkMarshalJSON_NoContext(b *testing.B) {
	err := errors.New(errors.CodeNotFound, "resource not found")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(err)
	}
}

func BenchmarkErrorString(b *testing.B) {
	err := errors.New(errors.CodeNotFound, "resource not found")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

func BenchmarkErrorString_WithCause(b *testing.B) {
	cause := stderrors.New("connection refused")
	err := errors.Wrap(cause, errors.CodeNetwork, "network error")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

// Parallel benchmarks for concurrent usage patterns.
func BenchmarkNew_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = errors.New(errors.CodeNotFound, "not found")
		}
	})
}

func BenchmarkWithContext_Parallel(b *testing.B) {
	baseErr := errors.New(errors.CodeInternal, "internal")

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = errors.WithContext(baseErr, "key", "value")
		}
	})
}

func BenchmarkIsRetryable_Parallel(b *testing.B) {
	err := errors.New(errors.CodeTimeout, "timeout")

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = errors.IsRetryable(err)
		}
	})
}

// Comparison benchmarks.
func BenchmarkComparison_StdErrorsNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = stderrors.New("error message")
	}
}

func BenchmarkComparison_PlatformNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.New(errors.CodeInternal, "error message")
	}
}

func BenchmarkComparison_StdErrorsWrap(b *testing.B) {
	baseErr := stderrors.New("base error")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = stderrors.Join(stderrors.New("wrapper"), baseErr)
	}
}

func BenchmarkComparison_PlatformWrap(b *testing.B) {
	baseErr := stderrors.New("base error")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = errors.Wrap(baseErr, errors.CodeInternal, "wrapper")
	}
}
