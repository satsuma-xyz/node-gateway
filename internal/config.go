package internal

import (
	"errors"
	"os"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type UpstreamConfig struct {
	ID      string `yaml:"id"`
	Chain   string `yaml:"chain"`
	HTTPURL string `yaml:"httpURL"`
	WSURL   string `yaml:"wsURL"`
	// Specifies whether to use a websocket subscription to `newHeads` for monitoring block height. Will fall back to HTTP polling if not set.
	UseWsForBlockHeight bool `yaml:"useWsForBlockHeight"`
}

func (c UpstreamConfig) isValid() bool {
	isValid := true
	if c.HTTPURL == "" {
		isValid = false

		zap.L().Error("httpUrl cannot be empty", zap.Any("config", c), zap.String("nodeId", c.ID))
	}

	if c.UseWsForBlockHeight && c.WSURL == "" {
		isValid = false

		zap.L().Error("wsURL should be provided if useWsForBlockHeight=true.", zap.Any("config", c), zap.String("nodeId", c.ID))
	}

	return isValid
}

type GlobalConfig struct {
	Port int `yaml:"port"`
}

type Config struct {
	Upstreams []UpstreamConfig
	Global    GlobalConfig
}

func LoadConfig(configFilePath string) (config Config, err error) {
	configBytes, err := os.ReadFile(configFilePath)

	if err != nil {
		return Config{}, err
	}

	config, err = parseConfig(configBytes)

	return config, err
}

func parseConfig(configBytes []byte) (config Config, err error) {
	config = Config{}

	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return config, err
	}

	isValid := true
	for _, upstream := range config.Upstreams {
		isValid = isValid && upstream.isValid()
	}

	if !isValid {
		err = errors.New("invalid config found")
	}

	return config, err
}
