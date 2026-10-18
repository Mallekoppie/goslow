package platform

import (
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Log is a stable handle for the process lifetime. Its underlying zap
	// logger is swapped atomically by ConfigureLoggerWithConfig, so concurrent
	// logging and reconfiguration are race free. Before the first configuration
	// every method is a safe no-op.
	Log = &Logger{}

	ErrInvalidLogLevel = errors.New("incorrect log level: unable to translate to zap log level")
)

type Logger struct {
	current atomic.Pointer[zap.Logger]
}

func (l *Logger) logger() *zap.Logger {
	if l == nil {
		return nil
	}
	return l.current.Load()
}

func (l *Logger) Info(msg string, fields ...zap.Field) {
	if logger := l.logger(); logger != nil {
		logger.Info(msg, fields...)
	}
}

func (l *Logger) Error(msg string, fields ...zap.Field) {
	if logger := l.logger(); logger != nil {
		logger.Error(msg, fields...)
	}
}

func (l *Logger) Debug(msg string, fields ...zap.Field) {
	if logger := l.logger(); logger != nil {
		logger.Debug(msg, fields...)
	}
}

func (l *Logger) Warn(msg string, fields ...zap.Field) {
	if logger := l.logger(); logger != nil {
		logger.Warn(msg, fields...)
	}
}

// InitializeLogger loads configuration (if needed) and configures the logger.
// It is idempotent and safe to call before any logging happens.
func InitializeLogger() error {
	cfg, err := GetPlatformConfiguration()
	if err != nil {
		return err
	}
	return ConfigureLoggerWithConfig(cfg)
}

// ConfigureLogger reconfigures the logger from the current platform config.
func ConfigureLogger() error {
	cfg, err := GetPlatformConfiguration()
	if err != nil {
		return err
	}
	return ConfigureLoggerWithConfig(cfg)
}

var configureLoggerLock sync.Mutex

// ConfigureLoggerWithConfig swaps the underlying logger atomically. It validates
// the configured level and returns ErrInvalidLogLevel instead of silently
// falling back.
func ConfigureLoggerWithConfig(cfg *Config) error {
	if cfg == nil {
		cfg = createDefaultConfiguration()
	}

	level, err := logLevelStringToZapType(cfg.Log.Level)
	if err != nil {
		return err
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	consoleWriter := zapcore.Lock(os.Stdout)
	zapCores := []zapcore.Core{zapcore.NewCore(jsonEncoder, consoleWriter, level)}

	if cfg.Log.FileLoggingEnabled {
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.Log.FilePath,
			MaxSize:    cfg.Log.MaxSize,
			MaxAge:     cfg.Log.MaxAge,
			MaxBackups: cfg.Log.MaxBackups,
		})
		zapCores = append(zapCores, zapcore.NewCore(jsonEncoder, fileWriter, level))
	}

	newLogger := zap.New(zapcore.NewTee(zapCores...), zap.AddCaller(), zap.AddCallerSkip(1))
	newLogger = newLogger.With(zap.Field{Key: "component", Type: zapcore.StringType, String: cfg.Component.ComponentName})

	configureLoggerLock.Lock()
	Log.current.Store(newLogger)
	configureLoggerLock.Unlock()
	return nil
}

func logLevelStringToZapType(input string) (zap.AtomicLevel, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return zap.NewAtomicLevelAt(zap.InfoLevel), nil
	}
	switch input {
	case "debug":
		return zap.NewAtomicLevelAt(zap.DebugLevel), nil
	case "info":
		return zap.NewAtomicLevelAt(zap.InfoLevel), nil
	case "warn", "warning":
		return zap.NewAtomicLevelAt(zap.WarnLevel), nil
	case "error":
		return zap.NewAtomicLevelAt(zap.ErrorLevel), nil
	case "fatal":
		return zap.NewAtomicLevelAt(zap.FatalLevel), nil
	case "panic":
		return zap.NewAtomicLevelAt(zap.PanicLevel), nil
	default:
		return zap.NewAtomicLevelAt(zap.InfoLevel), ErrInvalidLogLevel
	}
}
