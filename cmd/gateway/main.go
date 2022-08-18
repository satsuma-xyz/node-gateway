package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/satsuma-data/node-gateway/internal"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"go.uber.org/zap"
)

// The 1st arg is the path to the program and the 2nd arg is the path to the
// config file.
const ExpectedNumArgs = 2

func main() {
	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

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

	if len(os.Args) < ExpectedNumArgs {
		logger.Fatal("No config file specified.")
	}

	config, err := internal.LoadConfig(os.Args[1])
	if err != nil {
		zap.L().Fatal("Failed to load config.", zap.Error(err))
	}

	var rpcServer internal.RPCServer

	var metricsServer *http.Server

	zap.L().Info("Starting node gateway.", zap.String("env", env), zap.Any("config", config))

	go func() {
		rpcServer = internal.NewRPCServer(config)

		if err := rpcServer.Start(); err != http.ErrServerClosed {
			zap.L().Fatal("Failed to start RPC server.", zap.Error(err))
		}
	}()

	zap.L().Info("Starting metrics server.", zap.String("env", env), zap.Int("port", metrics.DefaultPort))

	go func() {
		metricsServer = metrics.NewMetricsServer()

		if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
			zap.L().Fatal("Failed to start metrics server.", zap.Error(err))
		}
	}()

	// Wait for an Unix exit signal.
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	exitSignal := <-signalChannel
	zap.L().Info("Exiting due to signal", zap.Any("signal", exitSignal))

	// Perform graceful shutdown.
	zap.L().Info("Shutting down")

	if err := rpcServer.Shutdown(); err != nil {
		zap.L().Fatal("Failed to gracefully shut down RPC server", zap.Error(err))
	}

	if err := metricsServer.Shutdown(context.Background()); err != nil {
		zap.L().Fatal("Failed to gracefully shut down metrics server", zap.Error(err))
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
