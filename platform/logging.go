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
	Logger             *zap.Logger
	ErrInvalidLogLevel = errors.New("Incorrect Log level. Unable to translate to ZAP log level")
)

func init() {
	InitializeLogger()
}

func InitializeLogger() {
	if Logger == nil {
		internaLoggerlLock.Lock()
		if Logger == nil {
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

			Logger = newLogger
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
