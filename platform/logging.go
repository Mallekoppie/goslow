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
	internaLoggerlLock sync.Mutex

	Log                *Logger
	ErrInvalidLogLevel = errors.New("incorrect log level: unable to translate to zap log level")
)

type Logger struct {
	internalLogger *zap.Logger
}

func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.internalLogger.Info(msg, fields...)
}

func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.internalLogger.Error(msg, fields...)
}

func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.internalLogger.Debug(msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.internalLogger.Warn(msg, fields...)
}

func init() {
	InitializeLogger()
}

func InitializeLogger() {
	if Log == nil {
		internaLoggerlLock.Lock()
		if Log == nil {
			log.Println("Creating new logger")

			platformConfig, err := GetPlatformConfiguration()
			if err != nil {
				log.Println("Unable to get Platform configuration: ", err.Error())
				panic(err.Error())
			}

			logLevel, err := logLevelStringToZapType(platformConfig.Log.Level)
			if err != nil {
				log.Fatalln("Unable to convert config log level to internal log level: ", err.Error())
			}

			// Log Rotation
			fileWriter := zapcore.AddSync(&lumberjack.Logger{
				Filename:   platformConfig.Log.FilePath,
				MaxSize:    platformConfig.Log.MaxSize,
				MaxAge:     platformConfig.Log.MaxAge,
				MaxBackups: platformConfig.Log.MaxBackups,
			})

			consoleWriter := zapcore.Lock(os.Stdout)

			encoderConfig := zap.NewProductionEncoderConfig()
			encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
			jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

			zapCores := []zapcore.Core{}
			// Only log to file if file logging is enabled
			if platformConfig.Log.FileLoggingEnabled {
				zapCores = append(zapCores, zapcore.NewCore(jsonEncoder, fileWriter, logLevel))
			}
			zapCores = append(zapCores, zapcore.NewCore(jsonEncoder, consoleWriter, logLevel))

			coreTree := zapcore.NewTee(zapCores...)

			newLogger := zap.New(coreTree, zap.AddCaller(), zap.AddCallerSkip(1))
			newLogger = newLogger.With(zap.Field{
				Key:    "component",
				Type:   zapcore.StringType,
				String: platformConfig.Component.ComponentName,
			})

			Log = &Logger{
				internalLogger: newLogger,
			}
		} else {
			internaLoggerlLock.Unlock()
		}
	}
}

func logLevelStringToZapType(input string) (zap.AtomicLevel, error) {
	input = strings.ToLower(input)
	var result zap.AtomicLevel

	switch input {
	case "debug":
		result = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		result = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		result = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		result = zap.NewAtomicLevelAt(zap.ErrorLevel)
	case "fatal":
		result = zap.NewAtomicLevelAt(zap.FatalLevel)
	case "panic":
		result = zap.NewAtomicLevelAt(zap.PanicLevel)
	default:
		result = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return result, nil
}
