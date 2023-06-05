package config

import (
	"errors"
	"os"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type NodeType string

const (
	Archive NodeType = "archive"
	Full    NodeType = "full"
)

type UpstreamConfig struct {
	Methods           MethodsConfig     `yaml:"methods"`
	HealthCheckConfig HealthCheckConfig `yaml:"healthCheck"`
	BasicAuthConfig   BasicAuthConfig   `yaml:"basicAuth"`
	ID                string            `yaml:"id"`
	HTTPURL           string            `yaml:"httpURL"`
	WSURL             string            `yaml:"wsURL"`
	GroupID           string            `yaml:"group"`
	NodeType          NodeType          `yaml:"nodeType"`
}

func (c *UpstreamConfig) isValid(groups []GroupConfig) bool {
	isValid := true
	if c.HTTPURL == "" {
		isValid = false

		zap.L().Error("httpUrl cannot be empty", zap.Any("config", c), zap.String("upstreamId", c.ID))
	}

	if c.NodeType == "" {
		isValid = false

		zap.L().Error("nodeType cannot be empty", zap.Any("config", c), zap.String("upstreamId", c.ID))
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

func IsUpstreamsValid(upstreams []UpstreamConfig) bool {
	var uniqueIDs = make(map[string]bool)
	for idx := range upstreams {
		if _, ok := uniqueIDs[upstreams[idx].ID]; ok {
			zap.L().Error("Upstream IDs should be unique across groups of the same chain.", zap.Any("group", upstreams[idx].GroupID), zap.Any("upstream", upstreams[idx].ID))

			return false
		}

		uniqueIDs[upstreams[idx].ID] = true
	}

	return true
}

type HealthCheckConfig struct {
	// If not set - method to identify block height is auto-detected. Use websockets is its URL is set, else fall back to use HTTP polling.
	UseWSForBlockHeight *bool `yaml:"useWsForBlockHeight"`
	SkipPeerCountCheck  *bool `yaml:"skipPeerCountCheck"`
}

type BasicAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type MethodsConfig struct {
	Enabled  map[string]bool `yaml:"enabled"`  // Emulating `Set` data structure
	Disabled map[string]bool `yaml:"disabled"` // Emulating `Set` data structure
}

func (m *MethodsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type MethodsConfigString struct {
		Enabled  []string
		Disabled []string
	}

	var methodsConfigString MethodsConfigString
	err := unmarshal(&methodsConfigString)

	if err != nil {
		return err
	}

	m.Enabled = make(map[string]bool)
	for _, method := range methodsConfigString.Enabled {
		m.Enabled[method] = true
	}

	m.Disabled = make(map[string]bool)
	for _, method := range methodsConfigString.Disabled {
		m.Disabled[method] = true
	}

	return nil
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

type RoutingConfig struct {
	MaxBlocksBehind int `yaml:"maxBlocksBehind"`
}

type SingleChainConfig struct {
	ChainName string `yaml:"chainName"`
	Upstreams []UpstreamConfig
	Groups    []GroupConfig
	Routing   RoutingConfig
}

func (c *SingleChainConfig) isValid() bool {
	isChainConfigValid := true
	isChainConfigValid = isChainConfigValid && IsGroupsValid(c.Groups)
	isChainConfigValid = isChainConfigValid && IsUpstreamsValid(c.Upstreams)

	for idx := range c.Upstreams {
		isChainConfigValid = isChainConfigValid && c.Upstreams[idx].isValid(c.Groups)
	}

	if c.ChainName == "" {
		zap.L().Error("chainName cannot be empty", zap.Any("config", c))

		isChainConfigValid = false
	}

	return isChainConfigValid
}

func isChainsValid(chainsConfig []SingleChainConfig) bool {
	isValid := len(chainsConfig) > 0

	for chainIndex := range chainsConfig {
		chainConfig := &chainsConfig[chainIndex]

		isValid = isValid && chainConfig.isValid()
	}

	return isValid
}

type Config struct {
	Chains []SingleChainConfig
	Global GlobalConfig
}

func (config *Config) Validate() error {
	isValid := isChainsValid(config.Chains)

	if !isValid {
		return errors.New("invalid config found")
	}

	return nil
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

	err = config.Validate()

	return config, err
}
