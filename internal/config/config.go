package config

import (
	"errors"
	"os"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type UpstreamConfig struct {
	BasicAuthConfig   BasicAuthConfig   `yaml:"basicAuth"`
	HealthCheckConfig HealthCheckConfig `yaml:"healthCheck"`
	ID                string            `yaml:"id"`
	HTTPURL           string            `yaml:"httpURL"`
	WSURL             string            `yaml:"wsURL"`
	GroupID           string            `yaml:"group"`
}

func (c *UpstreamConfig) isValid(groups []GroupConfig) bool {
	isValid := true
	if c.HTTPURL == "" {
		isValid = false

		zap.L().Error("httpUrl cannot be empty", zap.Any("config", c), zap.String("upstreamId", c.ID))
	}

	if c.HealthCheckConfig.UseWSForBlockHeight != nil && *c.HealthCheckConfig.UseWSForBlockHeight && c.WSURL == "" {
		isValid = false

		zap.L().Error("wsURL should be provided if useWsForBlockHeight=true.", zap.Any("config", c), zap.String("upstreamId", c.ID))
	}

	if len(groups) > 0 {
		if c.GroupID == "" {
			isValid = false

			zap.L().Error("A Group must be specified on upstreams since groups are defined.", zap.Any("config", c), zap.String("upstreamId", c.ID))
		} else {
			groupIsValid := false
			for _, group := range groups {
				if group.ID == c.GroupID {
					groupIsValid = true
				}
			}

			if !groupIsValid {
				isValid = false

				zap.L().Error("Invalid group specified on upstream.", zap.Any("config", c), zap.String("upstreamId", c.ID))
			}
		}
	}

	return isValid
}

type HealthCheckConfig struct {
	// If not set - method to identify block height is auto-detected. Use websockets is its URL is set, else fall back to use HTTP polling.
	UseWSForBlockHeight *bool `yaml:"useWsForBlockHeight"`
}

type BasicAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type GroupConfig struct {
	ID       string `yaml:"id"`
	Priority int    `yaml:"priority"`
}

func IsGroupsValid(groups []GroupConfig) bool {
	var uniqueIDs = make(map[string]bool)
	for _, group := range groups {
		if _, ok := uniqueIDs[group.ID]; ok {
			zap.L().Error("Group IDs should be unique.", zap.Any("group", group))

			return false
		}

		uniqueIDs[group.ID] = true
	}

	var uniquePriorities = make(map[int]bool)
	for _, group := range groups {
		if _, ok := uniquePriorities[group.Priority]; ok {
			zap.L().Error("Group priorities should be unique.", zap.Any("group", group))

			return false
		}

		uniquePriorities[group.Priority] = true
	}

	return true
}

type GlobalConfig struct {
	Port int `yaml:"port"`
}

type Config struct {
	Upstreams []UpstreamConfig
	Groups    []GroupConfig
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

	isValid := IsGroupsValid(config.Groups)

	for _, upstream := range config.Upstreams {
		isValid = isValid && upstream.isValid(config.Groups)
	}

	if !isValid {
		err = errors.New("invalid config found")
	}

	return config, err
}
