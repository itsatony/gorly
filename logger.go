package ratelimit

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ============================================================================
// LOGGER INTERFACE - Abstraction for logging
// ============================================================================

// Logger defines the logging interface for gorly
// This allows injection of any logging implementation
type Logger interface {
	// Debug logs a debug message with optional fields
	Debug(msg string, fields ...interface{})

	// Info logs an info message with optional fields
	Info(msg string, fields ...interface{})

	// Warn logs a warning message with optional fields
	Warn(msg string, fields ...interface{})

	// Error logs an error message with optional fields
	Error(msg string, fields ...interface{})

	// With creates a child logger with the given fields
	With(fields ...interface{}) Logger

	// Named creates a named logger
	Named(name string) Logger
}

// ============================================================================
// ZAP LOGGER IMPLEMENTATION - Default logger using uber/zap
// ============================================================================

// zapLogger wraps zap.Logger to implement our Logger interface
type zapLogger struct {
	logger *zap.Logger
}

// NewDevelopmentLogger creates a logger optimized for development
func NewDevelopmentLogger() Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return &zapLogger{logger: zap.NewNop()}
	}

	return &zapLogger{logger: logger}
}

// NewProductionLogger creates a logger optimized for production
func NewProductionLogger() Logger {
	logger, err := zap.NewProduction()
	if err != nil {
		return &zapLogger{logger: zap.NewNop()}
	}

	return &zapLogger{logger: logger}
}

// NewLoggerFromZap wraps an existing zap.Logger
func NewLoggerFromZap(logger *zap.Logger) Logger {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &zapLogger{logger: logger}
}

// NewCustomLogger creates a logger with custom configuration
func NewCustomLogger(level zapcore.Level, encoding string, outputPaths []string) Logger {
	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      false,
		Encoding:         encoding,
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      outputPaths,
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := config.Build()
	if err != nil {
		return &zapLogger{logger: zap.NewNop()}
	}

	return &zapLogger{logger: logger}
}

// Debug logs a debug message
func (zl *zapLogger) Debug(msg string, fields ...interface{}) {
	if len(fields) == 0 {
		zl.logger.Debug(msg)
		return
	}
	zl.logger.Debug(msg, convertToZapFields(fields)...)
}

// Info logs an info message
func (zl *zapLogger) Info(msg string, fields ...interface{}) {
	if len(fields) == 0 {
		zl.logger.Info(msg)
		return
	}
	zl.logger.Info(msg, convertToZapFields(fields)...)
}

// Warn logs a warning message
func (zl *zapLogger) Warn(msg string, fields ...interface{}) {
	if len(fields) == 0 {
		zl.logger.Warn(msg)
		return
	}
	zl.logger.Warn(msg, convertToZapFields(fields)...)
}

// Error logs an error message
func (zl *zapLogger) Error(msg string, fields ...interface{}) {
	if len(fields) == 0 {
		zl.logger.Error(msg)
		return
	}
	zl.logger.Error(msg, convertToZapFields(fields)...)
}

// With creates a child logger with the given fields
func (zl *zapLogger) With(fields ...interface{}) Logger {
	if len(fields) == 0 {
		return zl
	}
	return &zapLogger{
		logger: zl.logger.With(convertToZapFields(fields)...),
	}
}

// Named creates a named logger
func (zl *zapLogger) Named(name string) Logger {
	return &zapLogger{
		logger: zl.logger.Named(name),
	}
}

// ============================================================================
// NOP LOGGER IMPLEMENTATION - Silent logger for testing
// ============================================================================

// nopLogger is a no-operation logger that discards all logs
type nopLogger struct{}

// NewNopLogger creates a logger that discards all logs
// Useful for testing or when logging is disabled
func NewNopLogger() Logger {
	return &nopLogger{}
}

func (nl *nopLogger) Debug(msg string, fields ...interface{}) {}
func (nl *nopLogger) Info(msg string, fields ...interface{})  {}
func (nl *nopLogger) Warn(msg string, fields ...interface{})  {}
func (nl *nopLogger) Error(msg string, fields ...interface{}) {}
func (nl *nopLogger) With(fields ...interface{}) Logger       { return nl }
func (nl *nopLogger) Named(name string) Logger                { return nl }

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// convertToZapFields converts key-value pairs to zap.Field
// Expects fields to be in key-value pairs: key1, value1, key2, value2, ...
func convertToZapFields(fields []interface{}) []zap.Field {
	if len(fields) == 0 {
		return nil
	}

	zapFields := make([]zap.Field, 0, len(fields)/2)

	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}

		value := fields[i+1]
		zapFields = append(zapFields, zap.Any(key, value))
	}

	return zapFields
}

// LogField is a helper type for structured logging fields
type LogField struct {
	Key   string
	Value interface{}
}

// F creates a LogField (shorthand for structured logging)
func F(key string, value interface{}) LogField {
	return LogField{Key: key, Value: value}
}

// ConvertLogFields converts LogField slice to interface{} slice for logging
func ConvertLogFields(fields []LogField) []interface{} {
	if len(fields) == 0 {
		return nil
	}

	result := make([]interface{}, 0, len(fields)*2)
	for _, field := range fields {
		result = append(result, field.Key, field.Value)
	}
	return result
}
