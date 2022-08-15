package internal

import (
	"errors"
	"os"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type UpstreamConfig struct {
	ID                string            `yaml:"id"`
	Chain             string            `yaml:"chain"`
	HTTPURL           string            `yaml:"httpURL"`
	WSURL             string            `yaml:"wsURL"`
	HealthCheckConfig HealthCheckConfig `yaml:"healthCheckConfig"`
}

func (c UpstreamConfig) isValid() bool {
	isValid := true
	if c.HTTPURL == "" {
		isValid = false

		zap.L().Error("httpUrl cannot be empty", zap.Any("config", c), zap.String("nodeId", c.ID))
	}

	if c.HealthCheckConfig.UseWSForBlockHeight && c.WSURL == "" {
		isValid = false

		zap.L().Error("wsURL should be provided if useWsForBlockHeight=true.", zap.Any("config", c), zap.String("nodeId", c.ID))
	}

	return isValid
}

type HealthCheckConfig struct {
	UseWSForBlockHeight bool `yaml:"useWsForBlockHeight"`
}

type GlobalConfig struct {
	Port int `yaml:"port"`
}

type Config struct {
	Upstreams []UpstreamConfig
	Global    GlobalConfig
}

func LoadConfig(configFilePath string) (Config, error) {
	configBytes, err := os.ReadFile(configFilePath)

	if err != nil {
		return Config{}, err
	}

	return parseConfig(configBytes)
}

func parseConfig(configBytes []byte) (Config, error) {
	config := Config{}

	err := yaml.Unmarshal(configBytes, &config)
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
