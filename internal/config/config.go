package config //nolint:nolintlint,typecheck // Legacy

import (
	"errors"
	"os"
	"slices"
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
	DefaultMaxLatency                  = 10 * time.Second
	DefaultErrorRate                   = 0.25
	DefaultLatencyTooHighRate          = 0.5 // TODO(polsar): Expose this parameter in the config.
	Archive                   NodeType = "archive"
	Full                      NodeType = "full"
	// PassiveLatencyCheckMethod is a dummy method we use to measure the latency of an upstream RPC endpoint.
	// https://docs.infura.io/api/networks/ethereum/json-rpc-methods/eth_chainid
	PassiveLatencyCheckMethod = "eth_chainId" //nolint:gosec // No hardcoded credentials here.
	// PassiveLatencyChecking indicates whether to use live (active) requests as data for the LatencyChecker (false)
	// or synthetic (passive) periodic requests (true).
	// TODO(polsar): This setting is currently not configurable via the YAML config file.
	// TODO(polsar): We may also consider a hybrid request latency/error checking using both active and passive requests.
	PassiveLatencyChecking = false
)

// UnhealthyReason is the reason why a health check failed. We use it to select the most appropriate upstream to route to
// if all upstreams are unhealthy and the `alwaysRoute` option is true.
type UnhealthyReason int

const (
	ReasonUnknownOrHealthy = iota
	ReasonErrorRate
	ReasonLatencyTooHighRate
)

// UpstreamConfig
// TODO(polsar): Move the HealthStatus field to a new struct and embed this struct in it. Asana task: https://app.asana.com/0/1207397277805097/1208232039997185/f
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
	HealthStatus         UnhealthyReason       // The default value of this field is 0 (ReasonUnknownOrHealthy).
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
	Cache   CacheConfig   `yaml:"cache"`
	Routing RoutingConfig `yaml:"routing"`
	Port    int           `yaml:"port"`
}

// setDefaults sets the default values for the global config if global enhanced routing is specified in the YAML,
// and returns true. Otherwise, it does nothing and returns false.
func (c *GlobalConfig) setDefaults() bool {
	return c.Routing.setDefaults(nil, false)
}

type CacheConfig struct {
	Redis string `yaml:"redis"`
}

// ErrorsConfig
// TODO(polsar): Add the minimum number of requests in the detection window required to apply the error rate.
type ErrorsConfig struct {
	HTTPCodes    []string `yaml:"httpCodes"`
	JSONRPCCodes []string `yaml:"jsonRpcCodes"`
	ErrorStrings []string `yaml:"errorStrings"`
	Rate         float64  `yaml:"rate"`
}

func (c *ErrorsConfig) merge(globalConfig *ErrorsConfig) {
	if globalConfig == nil {
		return
	}

	// TODO(polsar): Can we somehow combine these three sections into one to avoid code duplication?
	c.HTTPCodes = append(c.HTTPCodes, globalConfig.HTTPCodes...)
	c.HTTPCodes = sortAndRemoveDuplicates(c.HTTPCodes)

	c.JSONRPCCodes = append(c.JSONRPCCodes, globalConfig.JSONRPCCodes...)
	c.JSONRPCCodes = sortAndRemoveDuplicates(c.JSONRPCCodes)

	c.ErrorStrings = append(c.ErrorStrings, globalConfig.ErrorStrings...)
	c.ErrorStrings = sortAndRemoveDuplicates(c.ErrorStrings)
}

func (c *ErrorsConfig) initialize(globalConfig *RoutingConfig) {
	var globalErrorsConfig *ErrorsConfig
	if globalConfig != nil {
		globalErrorsConfig = globalConfig.Errors
	}

	if c.Rate == 0 {
		if globalErrorsConfig == nil {
			c.Rate = DefaultErrorRate
		} else {
			c.Rate = globalErrorsConfig.Rate
		}
	}
}

// Sorts in-place and removes duplicates from the specified slice.
func sortAndRemoveDuplicates(s []string) []string {
	slices.Sort(s)
	return slices.Compact(s)
}

type MethodConfig struct {
	Name      string        `yaml:"method"`
	Threshold time.Duration `yaml:"threshold"`
}

func (c *MethodConfig) isMethodConfigValid(passiveLatencyChecking bool) bool {
	if passiveLatencyChecking {
		// TODO(polsar): Validate the method name: https://ethereum.org/en/developers/docs/apis/json-rpc/
		return !strings.EqualFold(c.Name, PassiveLatencyCheckMethod)
	}

	return true
}

// LatencyConfig
// TODO(polsar): Add the minimum number of latencies in the detection window required to apply the threshold.
// TODO(polsar): Add other aggregation options. Currently, the average of latencies in the detection windows is used.
type LatencyConfig struct {
	// This field allows us to quickly look up the latency of a method, rather than doing so by traversing the Methods slice.
	//
	// TODO(polsar): Move this field to a new struct and embed this struct in it.
	//  Asana task: https://app.asana.com/0/1207397277805097/1208232039997185/f
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
		var threshold time.Duration

		// The latency threshold is not configured or invalid, so use the global config's value or the default.
		if globalConfig != nil {
			threshold = globalConfig.getLatencyThreshold(nil)
		} else {
			threshold = DefaultMaxLatency
		}

		// The next time this method is called on the same LatencyConfig instance, this field will be set, and
		// we simply return its value.
		c.Threshold = threshold

		return threshold
	}

	return c.Threshold
}

func (c *LatencyConfig) getLatencyThresholdForMethod(method string) time.Duration {
	if latency, exists := c.MethodLatencyThresholds[method]; exists {
		return latency
	}

	return DefaultMaxLatency
}

func (c *LatencyConfig) initialize(globalConfig *RoutingConfig) {
	c.MethodLatencyThresholds = make(map[string]time.Duration)

	if c.Methods == nil {
		return
	}

	var globalLatencyConfig *LatencyConfig
	if globalConfig != nil {
		globalLatencyConfig = globalConfig.Latency
	}

	for _, method := range c.Methods {
		var threshold time.Duration

		if method.Threshold <= time.Duration(0) {
			// The method's latency threshold is not configured or invalid.
			if c.Threshold <= time.Duration(0) && globalLatencyConfig != nil {
				// Use the top-level value.
				threshold = globalLatencyConfig.getLatencyThresholdForMethod(method.Name)
			} else {
				// Use the global config latency value for the method.
				threshold = c.getLatencyThreshold(globalLatencyConfig)
			}
		} else {
			threshold = method.Threshold
		}

		c.MethodLatencyThresholds[method.Name] = threshold
	}
}

func (c *LatencyConfig) isLatencyConfigValid(passiveLatencyChecking bool) bool {
	if c == nil {
		return true
	}

	for _, method := range c.Methods {
		if !method.isMethodConfigValid(passiveLatencyChecking) {
			return false
		}
	}

	return true
}

type RoutingConfig struct {
	AlwaysRoute            *bool          `yaml:"alwaysRoute"`
	Errors                 *ErrorsConfig  `yaml:"errors"`
	Latency                *LatencyConfig `yaml:"latency"`
	DetectionWindow        *time.Duration `yaml:"detectionWindow"`
	BanWindow              *time.Duration `yaml:"banWindow"`
	MaxBlocksBehind        int            `yaml:"maxBlocksBehind"`
	PassiveLatencyChecking bool
	IsInitialized          bool
}

// HasEnhancedRoutingControlDefined returns true iff any of the enhanced routing control fields are specified
// in the config. Note that for the global routing config only, this method may return true even if global
// enhanced routing control is not defined in the YAML. This is because initializing per-chain routing config
// requires the global routing config to be initialized first.
func (r *RoutingConfig) HasEnhancedRoutingControlDefined() bool {
	// TODO(polsar): This is temporary. Eventually, we want to have enhanced routing control enabled by default even if
	// none of these fields are specified in the config YAML.
	return r.Errors != nil || r.Latency != nil || r.DetectionWindow != nil || r.BanWindow != nil || r.AlwaysRoute != nil
}

// setDefaults sets the default values for and initializes the routing config, and returns true.
// If enhanced routing control is disabled, it does nothing and returns false.
func (r *RoutingConfig) setDefaults(globalConfig *RoutingConfig, force bool) bool {
	if r.IsInitialized {
		return true
	}

	r.PassiveLatencyChecking = PassiveLatencyChecking

	if !force && !r.HasEnhancedRoutingControlDefined() && (globalConfig == nil || !globalConfig.HasEnhancedRoutingControlDefined()) {
		// Routing config is not specified at either this or global level, so there is nothing to do.
		return false
	}

	if globalConfig != nil && !globalConfig.IsInitialized {
		globalConfig.setDefaults(nil, true)
	}

	// For each routing config value that is not specified, use the corresponding global config value if the global config
	// is specified. Otherwise, use the default value. Note that if the global config is specified, it already has all
	// defaults set.

	if r.DetectionWindow == nil {
		if globalConfig == nil {
			r.DetectionWindow = NewDuration(DefaultDetectionWindow)
		} else {
			r.DetectionWindow = globalConfig.DetectionWindow
		}
	}

	if r.BanWindow == nil {
		if globalConfig == nil {
			r.BanWindow = NewDuration(DefaultBanWindow)
		} else {
			r.BanWindow = globalConfig.BanWindow
		}
	}

	if r.AlwaysRoute == nil {
		if globalConfig == nil {
			r.AlwaysRoute = new(bool) // &false
		} else {
			r.AlwaysRoute = globalConfig.AlwaysRoute
		}
	}

	if r.Latency == nil {
		if globalConfig == nil {
			r.Latency = new(LatencyConfig)
		} else {
			r.Latency = globalConfig.Latency
		}
	}

	r.Latency.initialize(globalConfig)

	if r.Errors == nil {
		if globalConfig == nil {
			r.Errors = new(ErrorsConfig)
		} else {
			r.Errors = globalConfig.Errors
		}
	}

	r.Errors.initialize(globalConfig)

	if globalConfig != nil {
		r.Latency.merge(globalConfig.Latency)
		r.Errors.merge(globalConfig.Errors)
	}

	r.IsInitialized = true

	return true
}

func (r *RoutingConfig) isRoutingConfigValid() bool {
	// TODO(polsar): Validate the HTTP and JSON RPC codes.
	isValid := r.isErrorRateValid()
	latency := r.Latency

	if latency != nil {
		isValid = isValid && latency.isLatencyConfigValid(r.PassiveLatencyChecking)
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

func (c *SingleChainConfig) setDefaults(globalConfig *GlobalConfig, isGlobalRoutingConfigSpecified bool) {
	if !isGlobalRoutingConfigSpecified && !c.Routing.HasEnhancedRoutingControlDefined() {
		c.Routing.PassiveLatencyChecking = PassiveLatencyChecking
		return
	}

	c.Routing.setDefaults(&globalConfig.Routing, false)
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

func (config *Config) setDefaults() {
	isGlobalRoutingConfigSpecified := config.Global.setDefaults()

	for chainIndex := range config.Chains {
		chainConfig := &config.Chains[chainIndex]
		chainConfig.setDefaults(&config.Global, isGlobalRoutingConfigSpecified)
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
