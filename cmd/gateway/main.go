package main

import (
	"os"

	"github.com/satsuma-data/node-gateway/internal/app/gateway"
	"go.uber.org/zap"
)

func main() {
	env := os.Getenv("ENV")
	logger, loggerErr := setupGlobalLogger(env)

	if loggerErr != nil {
		panic(loggerErr)
	}
	//nolint:errcheck // Ignore error from defer.
	defer logger.Sync() // Flushes buffer, if any.

	zap.L().Info("Starting node gateway.", zap.String("env", env))

	err := gateway.StartServer()
	if err != nil {
		logger.Fatal("Failed to start web server.", zap.Error(err))
	}
}

func setupGlobalLogger(env string) (logger *zap.Logger, err error) {
	if env == "production" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}

	if err == nil {
		zap.ReplaceGlobals(logger)
	}

	return logger, err
}
