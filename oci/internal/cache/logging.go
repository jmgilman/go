package cache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// LogLevel represents different logging levels
type LogLevel int

// LogLevelDebug represents debug logging level
const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// Logger provides structured logging for the cache system.
// It wraps different logger implementations for consistent behavior.
type Logger struct {
	impl loggerImpl
}

// loggerImpl defines the internal interface for logger implementations.
type loggerImpl interface {
	debug(ctx context.Context, msg string, args ...any)
	info(ctx context.Context, msg string, args ...any)
	warn(ctx context.Context, msg string, args ...any)
	error(ctx context.Context, msg string, args ...any)
	with(args ...any) *concreteLoggerImpl
}

// concreteLoggerImpl provides a concrete implementation that can be returned
type concreteLoggerImpl struct {
	impl loggerImpl
}

// nopLoggerWrapper is a special marker for nop logger with operations
type nopLoggerWrapper struct {
	*nopLogger
}

func (c *concreteLoggerImpl) debug(ctx context.Context, msg string, args ...any) {
	if c.impl != nil {
		c.impl.debug(ctx, msg, args...)
	}
}

func (c *concreteLoggerImpl) info(ctx context.Context, msg string, args ...any) {
	if c.impl != nil {
		c.impl.info(ctx, msg, args...)
	}
}

func (c *concreteLoggerImpl) warn(ctx context.Context, msg string, args ...any) {
	if c.impl != nil {
		c.impl.warn(ctx, msg, args...)
	}
}

func (c *concreteLoggerImpl) error(ctx context.Context, msg string, args ...any) {
	if c.impl != nil {
		c.impl.error(ctx, msg, args...)
	}
}

func (c *concreteLoggerImpl) with(args ...any) *concreteLoggerImpl {
	if c.impl != nil {
		return c.impl.with(args...)
	}
	return &concreteLoggerImpl{}
}

// Debug logs debug-level messages
func (l *Logger) Debug(ctx context.Context, msg string, args ...any) {
	if l.impl != nil {
		l.impl.debug(ctx, msg, args...)
	}
}

// Info logs info-level messages
func (l *Logger) Info(ctx context.Context, msg string, args ...any) {
	if l.impl != nil {
		l.impl.info(ctx, msg, args...)
	}
}

// Warn logs warning-level messages
func (l *Logger) Warn(ctx context.Context, msg string, args ...any) {
	if l.impl != nil {
		l.impl.warn(ctx, msg, args...)
	}
}

// Error logs error-level messages
func (l *Logger) Error(ctx context.Context, msg string, args ...any) {
	if l.impl != nil {
		l.impl.error(ctx, msg, args...)
	}
}

// With returns a logger with additional context fields
func (l *Logger) With(args ...any) *Logger {
	if l.impl == nil {
		return l
	}
	concreteImpl := l.impl.with(args...)
	// Special case for nop logger - return the same instance
	if _, ok := concreteImpl.impl.(*nopLoggerWrapper); ok {
		return l
	}
	return &Logger{impl: concreteImpl}
}

// WithOperation returns a logger with operation context
func (l *Logger) WithOperation(operation string) *Logger {
	return l.With("operation", operation)
}

// WithDigest returns a logger with digest context
func (l *Logger) WithDigest(digest string) *Logger {
	return l.With("digest", digest)
}

// WithSize returns a logger with size context
func (l *Logger) WithSize(size int64) *Logger {
	return l.With("size", size)
}

// WithDuration returns a logger with duration context
func (l *Logger) WithDuration(duration time.Duration) *Logger {
	return l.With("duration", duration)
}

// LogConfig holds configuration for the cache logger.
type LogConfig struct {
	// Level sets the minimum log level (debug, info, warn, error)
	Level LogLevel
	// EnableCallerInfo includes file and line number in logs
	EnableCallerInfo bool
	// EnablePerformanceLogging enables detailed performance metrics logging
	EnablePerformanceLogging bool
	// EnableCacheOperations enables logging of individual cache operations
	EnableCacheOperations bool
}

// DefaultLogConfig returns a default logging configuration.
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:                    LogLevelInfo,
		EnableCallerInfo:         false,
		EnablePerformanceLogging: true,
		EnableCacheOperations:    false, // Disabled by default to avoid noise
	}
}

// slogLogger implements the Logger interface using slog.
type slogLogger struct {
	logger *slog.Logger
	config LogConfig
	fields []any
}

// NewLogger creates a new structured logger with the given configuration.
func NewLogger(config LogConfig) *Logger {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: func() slog.Level {
			switch config.Level {
			case LogLevelDebug:
				return slog.LevelDebug
			case LogLevelInfo:
				return slog.LevelInfo
			case LogLevelWarn:
				return slog.LevelWarn
			case LogLevelError:
				return slog.LevelError
			default:
				return slog.LevelInfo
			}
		}(),
		AddSource: config.EnableCallerInfo,
	})

	return &Logger{
		impl: &slogLogger{
			logger: slog.New(handler),
			config: config,
			fields: make([]any, 0),
		},
	}
}

// NewNopLogger creates a no-op logger that discards all log messages.
func NewNopLogger() *Logger {
	return &Logger{
		impl: &nopLogger{},
	}
}

// debug logs debug-level messages.
func (l *slogLogger) debug(ctx context.Context, msg string, args ...any) {
	if l.config.Level <= LogLevelDebug {
		allArgs := make([]any, len(l.fields)+len(args))
		copy(allArgs, l.fields)
		copy(allArgs[len(l.fields):], args)
		l.logger.DebugContext(ctx, msg, allArgs...)
	}
}

// info logs info-level messages.
func (l *slogLogger) info(ctx context.Context, msg string, args ...any) {
	if l.config.Level <= LogLevelInfo {
		allArgs := make([]any, len(l.fields)+len(args))
		copy(allArgs, l.fields)
		copy(allArgs[len(l.fields):], args)
		l.logger.InfoContext(ctx, msg, allArgs...)
	}
}

// warn logs warning-level messages.
func (l *slogLogger) warn(ctx context.Context, msg string, args ...any) {
	if l.config.Level <= LogLevelWarn {
		allArgs := make([]any, len(l.fields)+len(args))
		copy(allArgs, l.fields)
		copy(allArgs[len(l.fields):], args)
		l.logger.WarnContext(ctx, msg, allArgs...)
	}
}

// error logs error-level messages.
func (l *slogLogger) error(ctx context.Context, msg string, args ...any) {
	if l.config.Level <= LogLevelError {
		allArgs := make([]any, len(l.fields)+len(args))
		copy(allArgs, l.fields)
		copy(allArgs[len(l.fields):], args)
		l.logger.ErrorContext(ctx, msg, allArgs...)
	}
}

// with returns a logger with additional context fields.
func (l *slogLogger) with(args ...any) *concreteLoggerImpl {
	newFields := make([]any, len(l.fields)+len(args))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], args)

	return &concreteLoggerImpl{
		impl: &slogLogger{
			logger: l.logger,
			config: l.config,
			fields: newFields,
		},
	}
}

// nopLogger is a no-op logger implementation that discards all messages.
type nopLogger struct{}

func (n *nopLogger) debug(ctx context.Context, msg string, args ...any) {}
func (n *nopLogger) info(ctx context.Context, msg string, args ...any)  {}
func (n *nopLogger) warn(ctx context.Context, msg string, args ...any)  {}
func (n *nopLogger) error(ctx context.Context, msg string, args ...any) {}
func (n *nopLogger) with(args ...any) *concreteLoggerImpl {
	return &concreteLoggerImpl{impl: &nopLoggerWrapper{nopLogger: n}}
}

// Operation represents different types of cache operations for logging.
type Operation string

// Operation constants for cache operations
const (
	OpGetManifest    Operation = "get_manifest"
	OpPutManifest    Operation = "put_manifest"
	OpGetBlob        Operation = "get_blob"
	OpPutBlob        Operation = "put_blob"
	OpDeleteEntry    Operation = "delete_entry"
	OpEvictEntry     Operation = "evict_entry"
	OpCleanupExpired Operation = "cleanup_expired"
	OpValidateDigest Operation = "validate_digest"
)

// LogCacheOperation logs a cache operation with performance metrics.
func LogCacheOperation(
	ctx context.Context,
	logger *Logger,
	operation Operation,
	duration time.Duration,
	success bool,
	size int64,
	err error,
) {
	if logger == nil {
		return
	}

	fields := []any{
		"operation", string(operation),
		"duration_ms", duration.Milliseconds(),
		"success", success,
	}

	if size > 0 {
		fields = append(fields, "size", size)
	}

	if err != nil {
		fields = append(fields, "error", err.Error())
	}

	if success {
		logger.Info(ctx, "cache operation completed", fields...)
	} else {
		logger.Warn(ctx, "cache operation failed", fields...)
	}
}

// LogCacheHit logs a cache hit event.
func LogCacheHit(ctx context.Context, logger *Logger, operation Operation, size int64) {
	if logger == nil {
		return
	}

	logger.Debug(ctx, "cache hit",
		"operation", string(operation),
		"size", size,
		"result", "hit")
}

// LogCacheMiss logs a cache miss event.
func LogCacheMiss(ctx context.Context, logger *Logger, operation Operation, reason string) {
	if logger == nil {
		return
	}

	logger.Debug(ctx, "cache miss",
		"operation", string(operation),
		"reason", reason,
		"result", "miss")
}

// LogEviction logs an eviction event.
func LogEviction(ctx context.Context, logger *Logger, key string, size int64, reason string) {
	if logger == nil {
		return
	}

	logger.Info(ctx, "cache entry evicted",
		"key", key,
		"size", size,
		"reason", reason)
}

// LogCleanup logs cleanup operations.
func LogCleanup(
	ctx context.Context,
	logger *Logger,
	operation string,
	entriesRemoved int,
	bytesFreed int64,
	duration time.Duration,
) {
	if logger == nil {
		return
	}

	logger.Info(ctx, "cache cleanup completed",
		"operation", operation,
		"entries_removed", entriesRemoved,
		"bytes_freed", bytesFreed,
		"duration_ms", duration.Milliseconds())
}

// LogPerformanceMetrics logs periodic performance metrics.
func LogPerformanceMetrics(ctx context.Context, logger *Logger, metrics *MetricsSnapshot) {
	if logger == nil {
		return
	}

	logger.Info(ctx, "cache performance metrics",
		"hit_rate", fmt.Sprintf("%.2f", metrics.HitRate),
		"hits", metrics.Hits,
		"misses", metrics.Misses,
		"evictions", metrics.Evictions,
		"errors", metrics.Errors,
		"bytes_stored", metrics.BytesStored,
		"entries_stored", metrics.EntriesStored,
		"bandwidth_saved", metrics.BandwidthSaved,
		"uptime", metrics.Uptime.String(),
	)
}

// ParseLogLevel parses a string log level into a LogLevel.
func ParseLogLevel(level string) (LogLevel, error) {
	switch strings.ToLower(level) {
	case "debug":
		return LogLevelDebug, nil
	case "info":
		return LogLevelInfo, nil
	case "warn", "warning":
		return LogLevelWarn, nil
	case "error":
		return LogLevelError, nil
	default:
		return LogLevelInfo, fmt.Errorf("invalid log level: %s", level)
	}
}
