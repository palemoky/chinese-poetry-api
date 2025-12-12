// Package logger provides a structured logging wrapper using zap.
package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// L is the global logger instance
	L    *zap.Logger
	once sync.Once
)

// Init initializes the global logger.
// If debug is true, uses development config with DEBUG level.
// Otherwise uses production config with INFO level.
func Init(debug bool) {
	once.Do(func() {
		var err error
		if debug {
			config := zap.NewDevelopmentConfig()
			config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
			L, err = config.Build()
		} else {
			config := zap.NewProductionConfig()
			config.EncoderConfig.TimeKey = "timestamp"
			config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
			L, err = config.Build()
		}
		if err != nil {
			// Fallback to nop logger if initialization fails
			L = zap.NewNop()
		}
	})
}

// Sync flushes any buffered log entries.
// Should be called before the application exits.
func Sync() {
	if L != nil {
		_ = L.Sync()
	}
}

// Default initializes a default logger if not already initialized.
func Default() *zap.Logger {
	if L == nil {
		Init(os.Getenv("GIN_MODE") != "release")
	}
	return L
}

// With creates a child logger with additional fields.
func With(fields ...zap.Field) *zap.Logger {
	return Default().With(fields...)
}

// Debug logs a debug message.
func Debug(msg string, fields ...zap.Field) {
	Default().Debug(msg, fields...)
}

// Info logs an info message.
func Info(msg string, fields ...zap.Field) {
	Default().Info(msg, fields...)
}

// Warn logs a warning message.
func Warn(msg string, fields ...zap.Field) {
	Default().Warn(msg, fields...)
}

// Error logs an error message.
func Error(msg string, fields ...zap.Field) {
	Default().Error(msg, fields...)
}

// Fatal logs a fatal message and exits.
func Fatal(msg string, fields ...zap.Field) {
	Default().Fatal(msg, fields...)
}
