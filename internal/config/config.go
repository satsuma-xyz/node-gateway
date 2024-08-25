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

const (
	DefaultBanWindow       = 5 * time.Minute
	DefaultDetectionWindow = time.Minute
	// DefaultMaxLatency is used when the latency threshold is not specified in the config.
	// TODO(polsar): We should probably use a lower value.
	DefaultMaxLatency          = 10 * time.Second
	Archive           NodeType = "archive"
	Full              NodeType = "full"
	// LatencyCheckMethod is a dummy method we use to measure the latency of an upstream RPC endpoint.
	// https://docs.infura.io/api/networks/ethereum/json-rpc-methods/eth_chainid
	LatencyCheckMethod = "eth_chainId"
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

func (c *GlobalConfig) initialize() {
	c.Routing.initialize(nil)
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

func (c *MethodConfig) isMethodConfigValid() bool {
	if c == nil {
		return true
	}
	// TODO(polsar): Validate the method name.
	return !strings.EqualFold(c.Name, LatencyCheckMethod)
}

type LatencyConfig struct {
	MethodLatencyThresholds map[string]time.Duration

	Methods   []MethodConfig `yaml:"methods"`
	Threshold time.Duration  `yaml:"threshold"`
}

func (c *LatencyConfig) merge(globalConfig *LatencyConfig) {
	if globalConfig == nil {
		return
	}

	for method, latencyThreshold := range globalConfig.MethodLatencyThresholds {
		if _, exists := c.MethodLatencyThresholds[method]; !exists {
			c.MethodLatencyThresholds[method] = latencyThreshold
		}
	}
}

func (c *LatencyConfig) getLatencyThreshold(globalConfig *LatencyConfig) time.Duration {
	if c.Threshold <= time.Duration(0) {
		// The latency threshold is not configured or invalid, so use the global config's value or the default.
		if globalConfig != nil {
			return globalConfig.getLatencyThreshold(nil)
		}

		return DefaultMaxLatency
	}

	return c.Threshold
}

func (c *LatencyConfig) getLatencyThresholdForMethod(method string, globalConfig *LatencyConfig) time.Duration {
	latency, exists := c.MethodLatencyThresholds[method]
	if !exists {
		// Use the global config's latency value or the default.
		if globalConfig != nil {
			return globalConfig.getLatencyThresholdForMethod(method, nil)
		}

		return DefaultMaxLatency
	}

	return latency
}

func (c *LatencyConfig) initialize(globalConfig *LatencyConfig) {
	c.MethodLatencyThresholds = make(map[string]time.Duration)

	if c.Methods == nil {
		return
	}

	for _, method := range c.Methods {
		var threshold time.Duration

		if method.Threshold <= time.Duration(0) {
			// The method's latency threshold is not configured or invalid
			if c.Threshold <= time.Duration(0) && globalConfig != nil {
				// Use the top-level value.
				threshold = globalConfig.getLatencyThresholdForMethod(method.Name, nil)
			} else {
				// Use the global config latency value for the method.
				threshold = c.getLatencyThreshold(globalConfig)
			}
		} else {
			threshold = method.Threshold
		}

		c.MethodLatencyThresholds[method.Name] = threshold
	}
}

func (c *LatencyConfig) isLatencyConfigValid() bool {
	if c == nil {
		return true
	}

	for _, method := range c.Methods {
		if !method.isMethodConfigValid() {
			return false
		}
	}

	return true
}

type RoutingConfig struct {
	AlwaysRoute     *bool          `yaml:"alwaysRoute"`
	Errors          *ErrorsConfig  `yaml:"errors"`
	Latency         *LatencyConfig `yaml:"latency"`
	DetectionWindow *time.Duration `yaml:"detectionWindow"`
	BanWindow       *time.Duration `yaml:"banWindow"`
	MaxBlocksBehind int            `yaml:"maxBlocksBehind"`
}

func (r *RoutingConfig) setDefaults() {
	if r.Errors == nil && r.Latency == nil {
		return
	}

	if r.DetectionWindow == nil {
		r.DetectionWindow = NewDuration(DefaultDetectionWindow)
	}

	if r.BanWindow == nil {
		r.BanWindow = NewDuration(DefaultBanWindow)
	}
}

func (r *RoutingConfig) initialize(globalConfig *RoutingConfig) {
	// TODO(polsar): Analogous code should be used from ErrorsConfig.
	var globalLatencyConfig *LatencyConfig

	if globalConfig != nil {
		globalLatencyConfig = globalConfig.Latency
	}

	if r.Latency == nil {
		if globalConfig != nil {
			// Use global latency config which is already initialized.
			r.Latency = globalLatencyConfig
		}

		return
	}

	r.Latency.initialize(globalLatencyConfig)

	if globalConfig != nil {
		// Merge global latency config into the chain latency config.
		r.Latency.merge(globalLatencyConfig)
	}
}

func (r *RoutingConfig) isRoutingConfigValid() bool {
	// TODO(polsar): Validate the HTTP and JSON RPC codes.
	isValid := r.isErrorRateValid()
	latency := r.Latency

	if latency != nil {
		isValid = isValid && latency.isLatencyConfigValid()
	}

	return isValid
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

func NewDuration(d time.Duration) *time.Duration {
	return &d
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

func (c *SingleChainConfig) initialize(globalConfig *GlobalConfig) {
	c.Routing.initialize(&globalConfig.Routing)
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

func (config *Config) initialize() {
	config.Global.initialize()

	for chainIndex := range config.Chains {
		chainConfig := &config.Chains[chainIndex]
		chainConfig.initialize(&config.Global)
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
	config.initialize()

	err = config.Validate()

	return config, err
}
