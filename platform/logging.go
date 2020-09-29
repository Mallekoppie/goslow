package platform

import (
	"errors"
	"log"
	"strings"
	"sync"

	"go.uber.org/zap"
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

			platformConfig, err := getPlatformConfiguration()
			if err != nil {
				log.Println("Unable to get Platform configuration: ", err.Error())
				panic(err.Error())
			}
			config := zap.NewProductionConfig()
			config.InitialFields = make(map[string]interface{}, 0)
			config.InitialFields["component"] = platformConfig.Component.ComponentName

			logLevel, err := logLevelStringToZapType(platformConfig.Log.Level)
			if err != nil {
				log.Println("Unable to convert config log level to internal log level: ", err.Error())
				panic(err.Error())
			}

			config.Level = logLevel
			config.OutputPaths = []string{"stderr", platformConfig.Log.FilePath}

			newLogger, err := config.Build()
			if err != nil {
				log.Println("Error while building logger: ", err.Error())
				panic(err.Error())
			}
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
		return result, ErrInvalidLogLevel
	}

	return result, nil
}
