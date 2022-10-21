package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The 1st arg is the path to the program and the 2nd arg is the path to the
// config file.
const (
	ExpectedNumArgs            = 2
	defaultLogLevelProduction  = zapcore.InfoLevel
	defaultLogLevelDevelopment = zapcore.DebugLevel
)

func main() {
	env := os.Getenv("ENV")
	logLevel := os.Getenv("LOG_LEVEL")
	logger, loggerErr := setupGlobalLogger(env, logLevel)

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

	conf, err := config.LoadConfig(os.Args[1])
	if err != nil {
		zap.L().Fatal("Failed to load config.", zap.Error(err))
	}

	var rpcServer *server.RPCServer

	var metricsServer *http.Server

	zap.L().Info("Starting node gateway.", zap.String("env", env), zap.Any("config", conf))

	go func() {
		dependencyContainer := server.WireDependenciesForAllChains(conf, logger)
		rpcServer = dependencyContainer.RPCServer

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
	zap.L().Info("Shutting down.")

	if err := rpcServer.Shutdown(); err != nil {
		zap.L().Fatal("Failed to gracefully shut down RPC server.", zap.Error(err))
	}

	if err := metricsServer.Shutdown(context.Background()); err != nil {
		zap.L().Fatal("Failed to gracefully shut down metrics server.", zap.Error(err))
	}
}

func setupGlobalLogger(env, logLevel string) (logger *zap.Logger, err error) {
	if env == "" || env == "production" {
		var parsedLogLevel zapcore.Level

		if logLevel != "" {
			if parsedLogLevel, err = zapcore.ParseLevel(logLevel); err != nil {
				parsedLogLevel = defaultLogLevelProduction
			}
		} else {
			parsedLogLevel = defaultLogLevelProduction
		}

		zapConfig := zap.NewProductionConfig()
		zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		zapConfig.Level = zap.NewAtomicLevelAt(parsedLogLevel)
		logger, err = zapConfig.Build()
	} else {
		var parsedLogLevel zapcore.Level

		if logLevel != "" {
			if parsedLogLevel, err = zapcore.ParseLevel(logLevel); err != nil {
				parsedLogLevel = defaultLogLevelDevelopment
			}
		} else {
			parsedLogLevel = defaultLogLevelDevelopment
		}

		zapConfig := zap.NewDevelopmentConfig()
		zapConfig.Level = zap.NewAtomicLevelAt(parsedLogLevel)
		logger, err = zapConfig.Build()
	}

	if err == nil {
		zap.ReplaceGlobals(logger)
	}

	return logger, err
}
