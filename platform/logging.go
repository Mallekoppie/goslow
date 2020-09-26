package platform

import (
	"errors"
	"log"
	"strings"
	"sync"

	"go.uber.org/zap"
)

var (
	internalLock       sync.Mutex
	logger             *zap.Logger
	ErrInvalidLogLevel = errors.New("Incorrect Log level. Unable to translate to ZAP log level")
)

func getLogger(componentName string, logLevel string, logFilePath string) (*zap.Logger, error) {
	internalLock.Lock()
	if logger == nil {
		log.Println("Creating new logger")
		config := zap.NewProductionConfig()
		config.InitialFields = make(map[string]interface{}, 0)
		config.InitialFields["component"] = componentName
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		config.OutputPaths = []string{"stderr", "./log.txt"}

		newLogger, err := config.Build()
		if err != nil {
			internalLock.Unlock()
			return nil, err
		}
		logger = newLogger
	}
	internalLock.Unlock()

	return logger, nil
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
