package config //nolint:nolintlint,typecheck // Legacy

import (
	"errors"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type NodeType string

const DefDetectionWindow = time.Minute
const DefBanWindow = 5 * time.Minute

const (
	Archive NodeType = "archive"
	Full    NodeType = "full"
)

type UpstreamConfig struct {
	Methods              MethodsConfig         `yaml:"methods"`
	HealthCheckConfig    HealthCheckConfig     `yaml:"healthCheck"`
	BasicAuthConfig      BasicAuthConfig       `yaml:"basicAuth"`
	ID                   string                `yaml:"id"`
	HTTPURL              string                `yaml:"httpURL"`
	WSURL                string                `yaml:"wsURL"`
	GroupID              string                `yaml:"group"`
	NodeType             NodeType              `yaml:"nodeType"`
	RequestHeadersConfig []RequestHeaderConfig `yaml:"requestHeaders"`
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
			for _, group := range groups { //nolint:nolintlint,wsl // Legacy
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

type RequestHeaderConfig struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

type MethodsConfig struct {
	Enabled  map[string]bool `yaml:"enabled"`  // Emulating `Set` data structure
	Disabled map[string]bool `yaml:"disabled"` // Emulating `Set` data structure
}

func (m *MethodsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type MethodsConfigString struct {
		Enabled  string
		Disabled string
	}

	var methodsConfigString MethodsConfigString
	err := unmarshal(&methodsConfigString)

	if err != nil {
		return err
	}

	m.Enabled = make(map[string]bool)
	for _, method := range strings.Split(methodsConfigString.Enabled, ",") {
		m.Enabled[method] = true
	}

	m.Disabled = make(map[string]bool)
	for _, method := range strings.Split(methodsConfigString.Disabled, ",") {
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
	Routing RoutingConfig `yaml:"routing"`
	Cache   CacheConfig   `yaml:"cache"`
	Port    int           `yaml:"port"`
}

func (c *GlobalConfig) setDefaults() {
	c.Routing.setDefaults()
}

type CacheConfig struct {
	Redis string `yaml:"redis"`
}

type ErrorsConfig struct {
	HTTPCodes    []string `yaml:"httpCodes"`
	JSONRPCCodes []string `yaml:"jsonRpcCodes"`
	ErrorStrings []string `yaml:"errorStrings"`
	Rate         float64  `yaml:"rate"`
}

type MethodConfig struct {
	Name      string        `yaml:"method"`
	Threshold time.Duration `yaml:"threshold"`
}

type LatencyConfig struct {
	Methods   []MethodConfig `yaml:"methods"`
	Threshold time.Duration  `yaml:"threshold"`
}

type RoutingConfig struct {
	AlwaysRoute     *bool          `yaml:"alwaysRoute"`
	Errors          *ErrorsConfig  `yaml:"errors"`
	Latency         *LatencyConfig `yaml:"latency"`
	DetectionWindow *time.Duration `yaml:"detectionWindow"`
	BanWindow       *time.Duration `yaml:"banWindow"`
	MaxBlocksBehind int            `yaml:"maxBlocksBehind"`
}

func newDuration(d time.Duration) *time.Duration {
	return &d
}

func (r *RoutingConfig) setDefaults() {
	if r.Errors == nil && r.Latency == nil {
		return
	}

	if r.DetectionWindow == nil {
		r.DetectionWindow = newDuration(DefDetectionWindow)
	}

	if r.BanWindow == nil {
		r.BanWindow = newDuration(DefBanWindow)
	}
}

func (r *RoutingConfig) isRoutingConfigValid() bool {
	// TODO(polsar): Validate the HTTP and JSON RPC codes, and potentially methods as well.
	return r.isErrorRateValid()
}

func (r *RoutingConfig) isErrorRateValid() bool {
	if r.Errors == nil {
		return true
	}

	rate := r.Errors.Rate
	isValid := 0.0 <= rate && rate <= 1.0

	if !isValid {
		zap.L().Error("Rate is not in range [0.0, 1.0]", zap.Any("rate", rate))
	}

	return isValid
}

type ChainCacheConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

func (c *ChainCacheConfig) isValid() bool {
	// The redis-cache library will default the TTL to 1 hour
	// if 0 < ttl < 1 second.
	if c.TTL > 0 && c.TTL < time.Second {
		zap.L().Error("ttl must be greater or equal to 1s")
		return false
	}

	return true
}

type SingleChainConfig struct {
	Routing   RoutingConfig
	ChainName string `yaml:"chainName"`
	Upstreams []UpstreamConfig
	Groups    []GroupConfig
	Cache     ChainCacheConfig
}

func (c *SingleChainConfig) isValid() bool {
	isChainConfigValid := IsGroupsValid(c.Groups)
	isChainConfigValid = isChainConfigValid && IsUpstreamsValid(c.Upstreams)
	isChainConfigValid = isChainConfigValid && c.Cache.isValid()
	isChainConfigValid = isChainConfigValid && c.Routing.isRoutingConfigValid()

	for idx := range c.Upstreams {
		isChainConfigValid = isChainConfigValid && c.Upstreams[idx].isValid(c.Groups)
	}

	if c.ChainName == "" {
		zap.L().Error("chainName cannot be empty", zap.Any("config", c))

		isChainConfigValid = false
	}

	return isChainConfigValid
}

func (c *SingleChainConfig) setDefaults() {
	c.Routing.setDefaults()
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
	Global GlobalConfig
	Chains []SingleChainConfig
}

func (config *Config) setDefaults() {
	config.Global.setDefaults()

	for chainIndex := range config.Chains {
		chainConfig := &config.Chains[chainIndex]
		chainConfig.setDefaults()
	}
}

func (config *Config) Validate() error {
	isValid := isChainsValid(config.Chains)

	// Validate global config.
	isValid = isValid && config.Global.Routing.isRoutingConfigValid()

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

	config.setDefaults()

	err = config.Validate()

	return config, err
}
