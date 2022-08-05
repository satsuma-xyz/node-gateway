package main

import (
	"os"

	"go.uber.org/zap"
)

func main() {
	env := os.Getenv("ENV")
	logger := setupLogger(env)

	//nolint:errcheck // Ignore error from defer.
	defer logger.Sync() // Flushes buffer, if any.

	logger.Info("Starting node gateway.", zap.String("env", env))
}

func setupLogger(env string) *zap.Logger {
	var (
		logger *zap.Logger
		err    error
	)

	if env == "production" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}

	if err != nil {
		logger.Fatal("failed to create logger.", zap.Error(err))
	}

	return logger
}
