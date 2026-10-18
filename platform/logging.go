package platform

import (
	"errors"
	"log"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	loggerLock         sync.Mutex
	Log                *Logger
	ErrInvalidLogLevel = errors.New("incorrect log level: unable to translate to zap log level")
)

type Logger struct {
	internalLogger *zap.Logger
}

func (l *Logger) Info(msg string, fields ...zap.Field)  { l.internalLogger.Info(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.internalLogger.Error(msg, fields...) }
func (l *Logger) Debug(msg string, fields ...zap.Field) { l.internalLogger.Debug(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.internalLogger.Warn(msg, fields...) }

func init() {
	InitializeLogger()
}

func InitializeLogger() {
	loggerLock.Lock()
	defer loggerLock.Unlock()
	if Log != nil {
		return
	}
	cfg, err := GetPlatformConfiguration()
	if err != nil {
		log.Println("Unable to get Platform configuration: ", err.Error())
		cfg = createDefaultConfiguration()
	}
	ConfigureLoggerWithConfig(cfg)
}

func ConfigureLogger() {
	loggerLock.Lock()
	defer loggerLock.Unlock()
	cfg, err := GetPlatformConfiguration()
	if err != nil || cfg == nil {
		cfg = createDefaultConfiguration()
	}
	ConfigureLoggerWithConfig(cfg)
}

func ConfigureLoggerWithConfig(cfg *Config) {
	if cfg == nil {
		cfg = createDefaultConfiguration()
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	logLevel, err := logLevelStringToZapType(cfg.Log.Level)
	if err != nil {
		logLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	consoleWriter := zapcore.Lock(os.Stdout)
	zapCores := []zapcore.Core{zapcore.NewCore(jsonEncoder, consoleWriter, logLevel)}
	if cfg.Log.FileLoggingEnabled {
		fileWriter := zapcore.AddSync(&lumberjack.Logger{Filename: cfg.Log.FilePath, MaxSize: cfg.Log.MaxSize, MaxAge: cfg.Log.MaxAge, MaxBackups: cfg.Log.MaxBackups})
		zapCores = append(zapCores, zapcore.NewCore(jsonEncoder, fileWriter, logLevel))
	}
	newLogger := zap.New(zapcore.NewTee(zapCores...), zap.AddCaller(), zap.AddCallerSkip(1))
	newLogger = newLogger.With(zap.Field{Key: "component", Type: zapcore.StringType, String: cfg.Component.ComponentName})
	Log = &Logger{internalLogger: newLogger}
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
