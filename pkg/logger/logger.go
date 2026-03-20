package logger

import (
	"github.com/AlxPolt/sw-engineer-challenge/pkg/sanitizer"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
}

func New(service, level string) (*Logger, error) {
	zapLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		zapLevel = zapcore.InfoLevel
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	cfg.InitialFields = map[string]interface{}{"service": service}

	zapLogger, err := cfg.Build(
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return &Logger{zapLogger}, nil
}

func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	l.Logger.Sugar().Infow(msg, sanitizeKV(keysAndValues)...)
}

func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	l.Logger.Sugar().Debugw(msg, sanitizeKV(keysAndValues)...)
}

func (l *Logger) Warn(msg string, keysAndValues ...interface{}) {
	l.Logger.Sugar().Warnw(msg, sanitizeKV(keysAndValues)...)
}

func (l *Logger) Error(msg string, keysAndValues ...interface{}) {
	l.Logger.Sugar().Errorw(msg, sanitizeKV(keysAndValues)...)
}

func (l *Logger) Fatal(msg string, keysAndValues ...interface{}) {
	l.Logger.Sugar().Fatalw(msg, sanitizeKV(keysAndValues)...)
}

func (l *Logger) With(keysAndValues ...interface{}) *Logger {
	return &Logger{l.Logger.With(sanitizeFields(keysAndValues)...)}
}

func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return &Logger{l.Logger.With(zap.String("error", sanitizer.SanitizeError(err)))}
}

func sanitizeKV(kv []interface{}) []interface{} {
	if len(kv) == 0 {
		return kv
	}
	out := make([]interface{}, len(kv))
	for i, v := range kv {
		if i%2 == 0 {
			out[i] = v
			continue
		}
		switch val := v.(type) {
		case string:
			out[i] = sanitizer.Sanitize(val)
		case error:
			out[i] = sanitizer.SanitizeError(val)
		default:
			out[i] = v
		}
	}
	return out
}

func sanitizeFields(kv []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			continue
		}
		switch val := kv[i+1].(type) {
		case string:
			fields = append(fields, zap.String(key, sanitizer.Sanitize(val)))
		case error:
			fields = append(fields, zap.String(key, sanitizer.SanitizeError(val)))
		default:
			fields = append(fields, zap.Any(key, val))
		}
	}
	return fields
}
