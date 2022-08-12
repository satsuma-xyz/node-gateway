package main

import (
	"fmt"
	"os"

	"github.com/satsuma-data/node-gateway/internal"
	"go.uber.org/zap"
)

func main() {
	env := os.Getenv("ENV")
	logger, loggerErr := setupGlobalLogger(env)

	if loggerErr != nil {
		panic(loggerErr)
	}

	defer func() {
		// Flushes buffer, if any.
		err := logger.Sync()
		if err != nil {
			// There could be something wrong with the logger if it's not Syncing, so
			// print using `fmt.Println`.
			fmt.Println("Failed to sync logger.", zap.Error(err))
		}
	}()

	config, err := internal.LoadConfig(os.Args[1])
	if err != nil {
		logger.Fatal("Failed to load config.", zap.Error(err))
	}

	zap.L().Info("Starting node gateway.", zap.String("env", env), zap.Any("config", config))

	err = internal.StartServer(config)
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
