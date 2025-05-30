package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp" //nolint:imports // Legacy
	"github.com/stretchr/testify/assert"
)

func TestParseConfig_InvalidConfigs(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		config string
	}{
		{
			name: "Cache config has 0 < ttl < 1s",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                cache:
                  ttl: 0.5s
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    nodeType: full
            `,
		},
		{
			name: "Cache config has 0 < ttl < 1s",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                cache:
                  ttl: 10s
                  methods:
                    - method: eth_getLogs
                      ttl: 0.5s
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    nodeType: full
            `,
		},
		{
			name: "Upstream config without httpURL.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    wsURL: "wss://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    healthCheck:
                      useWsForBlockHeight: true
                    nodeType: full
            `,
		},
		{
			name: "Upstream config without nodeType.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            `,
		},
		{
			name: "UpstreamConfig without wssURL when useWsForBlockHeight: true.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    healthCheck:
                      useWsForBlockHeight: true
                    nodeType: full
            `,
		},
		{
			name: "Groups with same priority.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    group: primary
                    nodeType: full
                
                groups:
                  - id: primary
                    priority: 0
                  - id: fallback
                    priority: 0
            `,
		},
		{
			name: "Groups block defined but upstream does not declare group.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    nodeType: full
                
                groups:
                  - id: primary
                    priority: 0
            `,
		},
		{
			name: "Group name on upstream does not exist.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    group: something-that-doesnt-exist
                    nodeType: full
                
                groups:
                  - id: primary
                    priority: 0
            `,
		},
		{
			name: "Group with duplicate upstream names.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    group: primary
                    nodeType: full
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    group: primary
                    nodeType: full
                
                groups:
                  - id: primary
                    priority: 0
            `,
		},
		{
			name: "Duplicate upstream names across groups of the same chain.",
			config: `
            global:
              port: 8080

            chains:
              - chainName: ethereum
                upstreams:
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    group: primary
                    nodeType: full
                  - id: alchemy-eth
                    httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                    group: fallback
                    nodeType: full

                groups:
                  - id: primary
                    priority: 0
                  - id: fallback
                    priority: 1
            `,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configBytes := []byte(testCase.config)
			_, err := parseConfig(configBytes)
			assert.NotNil(t, err)

			// To prevent catching formatting errors, that's not what we're checking for in this test.
			if err != nil {
				assert.NotContains(t, err.Error(), "found character that cannot start any token", testCase.config)
				assert.NotContains(t, err.Error(), "found a tab character that violates indentation", testCase.config)
			}
		})
	}
}

func newBool(b bool) *bool {
	return &b
}

func TestParseConfig_ValidConfig(t *testing.T) {
	config := `
    global:
      port: 8080
      cache:
        redis: localhost:6379

    chains:
      - chainName: ethereum

        groups:
          - id: primary
            priority: 0
          - id: fallback
            priority: 1

        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            wsURL: "wss://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            healthCheck:
              useWsForBlockHeight: true
            group: primary
            nodeType: full
            methods:
              enabled: eth_getStorageAt
              disabled: eth_getBalance,getLogs
          - id: ankr-polygon
            httpURL: "https://rpc.ankr.com/polygon"
            wsURL: "wss://rpc.ankr.com/polygon/ws/${ANKR_API_KEY}"
            group: fallback
            nodeType: archive
            requestHeaders:
              - key: "x-api-key"
                value: "xxxx"
              - key: "client-id"
                value: "my-client"

      - chainName: polygon
        upstreams:
          - id: erigon-polygon-1
            httpURL: "http://127.0.0.1:4040"
            nodeType: archive
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Port: 8080,
			Cache: CacheConfig{
				Redis: "localhost:6379",
			},
		},
		Chains: []SingleChainConfig{{
			Upstreams: []UpstreamConfig{
				{
					ID:      "alchemy-eth",
					HTTPURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}",
					WSURL:   "wss://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}",
					HealthCheckConfig: HealthCheckConfig{
						UseWSForBlockHeight: newBool(true),
					},
					GroupID:  "primary",
					NodeType: Full,
					Methods: MethodsConfig{
						Enabled:  map[string]bool{"eth_getStorageAt": true},
						Disabled: map[string]bool{"eth_getBalance": true, "getLogs": true},
					},
				},
				{
					ID:      "ankr-polygon",
					HTTPURL: "https://rpc.ankr.com/polygon",
					WSURL:   "wss://rpc.ankr.com/polygon/ws/${ANKR_API_KEY}",
					HealthCheckConfig: HealthCheckConfig{
						UseWSForBlockHeight: nil,
					},
					GroupID:  "fallback",
					NodeType: Archive,
					RequestHeadersConfig: []RequestHeaderConfig{
						{
							Key:   "x-api-key",
							Value: "xxxx",
						},
						{
							Key:   "client-id",
							Value: "my-client",
						},
					},
				},
			},
			ChainName: "ethereum",
			Groups: []GroupConfig{
				{
					ID:       "primary",
					Priority: 0,
				},
				{
					ID:       "fallback",
					Priority: 1,
				},
			},
		}, {
			ChainName: "polygon",
			Upstreams: []UpstreamConfig{{
				ID:       "erigon-polygon-1",
				HTTPURL:  "http://127.0.0.1:4040",
				NodeType: Archive,
			}},
		}},
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func getCommonChainsConfig(routingConfig *RoutingConfig) []SingleChainConfig {
	c := RoutingConfig{}

	if routingConfig != nil {
		c = *routingConfig
	}

	return []SingleChainConfig{{
		Routing: c,
		Upstreams: []UpstreamConfig{
			{
				ID:       "alchemy-eth",
				HTTPURL:  "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}",
				GroupID:  "primary",
				NodeType: Full,
			},
		},
		ChainName: "ethereum",
		Groups: []GroupConfig{
			{
				ID:       "primary",
				Priority: 0,
			},
		},
	}}
}

func TestParseConfig_ValidConfigLatencyRouting_AllFieldsSet(t *testing.T) {
	config := `
    global:
      routing:
        maxBlocksBehind: 33
        detectionWindow: 10m
        banWindow: 50m
        errors:
          rate: 0.25
          httpCodes:
            - 5xx
            - 420
          jsonRpcCodes:
            - 32xxx
          errorStrings:
            - "internal server error"
        latency:
          threshold: 1000ms
          methods:
            - method: eth_getLogs
              threshold: 2000ms
            - method: eth_call
              threshold: 10000ms
        alwaysRoute: true

    chains:
      - chainName: ethereum

        groups:
          - id: primary
            priority: 0

        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedLatencyConfig := LatencyConfig{
		MethodLatencyThresholds: map[string]time.Duration{
			"eth_call":    10000 * time.Millisecond,
			"eth_getLogs": 2000 * time.Millisecond,
		},
		Threshold: 1000 * time.Millisecond,
		Methods: []MethodConfig{
			{
				Name:      "eth_getLogs",
				Threshold: 2000 * time.Millisecond,
			},
			{
				Name:      "eth_call",
				Threshold: 10000 * time.Millisecond,
			},
		},
	}

	expectedRoutingConfig := RoutingConfig{
		MaxBlocksBehind: 33,
		DetectionWindow: NewDuration(10 * time.Minute),
		BanWindow:       NewDuration(50 * time.Minute),
		Errors: &ErrorsConfig{
			Rate: 0.25,
			HTTPCodes: []string{
				"420",
				"5xx",
			},
			JSONRPCCodes: []string{
				"32xxx",
			},
			ErrorStrings: []string{
				"internal server error",
			},
		},
		Latency:       &expectedLatencyConfig,
		AlwaysRoute:   newBool(true),
		IsInitialized: true,
		IsEnabled:     true,
	}

	expectedRoutingChainConfig := expectedRoutingConfig
	expectedRoutingChainConfig.MaxBlocksBehind = 0

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: expectedRoutingConfig,
		},
		Chains: getCommonChainsConfig(&expectedRoutingChainConfig),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_ValidConfigLatencyRouting_ErrorsConfigOverridesAndMerges(t *testing.T) {
	config := `
    global:
      routing:
        errors:
          rate: 0.28
          httpCodes:
            - 5xx
            - 420
          jsonRpcCodes:
            - 32xxx
            - 28282
          errorStrings:
            - "internal server error"
            - "freaking out"

    chains:
      - chainName: ethereum

        groups:
          - id: primary
            priority: 0

        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full

        routing:
          errors:
            rate: 0.82
            httpCodes:
              - 5xx
              - 388
              - 4X4
            jsonRpcCodes:
              - XXXX1
              - 28282
            errorStrings:
              - "internal server error"
              - "some weird error"
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedRoutingConfig := RoutingConfig{
		DetectionWindow: NewDuration(DefaultDetectionWindow),
		BanWindow:       NewDuration(DefaultBanWindow),
		Errors: &ErrorsConfig{
			Rate: 0.28,
			HTTPCodes: []string{
				"5xx",
				"420",
			},
			JSONRPCCodes: []string{
				"32xxx",
				"28282",
			},
			ErrorStrings: []string{
				"internal server error",
				"freaking out",
			},
		},
		Latency:       &LatencyConfig{MethodLatencyThresholds: map[string]time.Duration{}},
		AlwaysRoute:   newBool(false),
		IsInitialized: true,
		IsEnabled:     true,
	}

	expectedRoutingChainConfig := expectedRoutingConfig
	expectedRoutingChainConfig.MaxBlocksBehind = 0
	expectedRoutingChainConfig.Errors = &ErrorsConfig{
		Rate: 0.82,
		HTTPCodes: []string{
			"388",
			"420",
			"4X4",
			"5xx",
		},
		JSONRPCCodes: []string{
			"28282",
			"32xxx",
			"XXXX1",
		},
		ErrorStrings: []string{
			"freaking out",
			"internal server error",
			"some weird error",
		},
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: expectedRoutingConfig,
		},
		Chains: getCommonChainsConfig(&expectedRoutingChainConfig),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_ValidConfigLatencyRouting_DefaultsForDetectionAndBanWindows_Set(t *testing.T) {
	config := `
    global:
      routing:
        errors:
          rate: 0.25
        latency:
          threshold: 1000ms

    chains:
      - chainName: ethereum
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedLatencyConfig := LatencyConfig{
		MethodLatencyThresholds: map[string]time.Duration{},
		Threshold:               1000 * time.Millisecond,
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: RoutingConfig{
				DetectionWindow: NewDuration(DefaultDetectionWindow),
				BanWindow:       NewDuration(DefaultBanWindow),
				Errors: &ErrorsConfig{
					Rate: 0.25,
				},
				Latency:       &expectedLatencyConfig,
				AlwaysRoute:   newBool(false),
				IsInitialized: true,
				IsEnabled:     true,
			},
		},
		Chains: getCommonChainsConfig(&RoutingConfig{
			DetectionWindow: NewDuration(DefaultDetectionWindow),
			BanWindow:       NewDuration(DefaultBanWindow),
			Errors:          &ErrorsConfig{Rate: DefaultErrorRate},
			Latency:         &expectedLatencyConfig,
			AlwaysRoute:     newBool(false),
			IsInitialized:   true,
			IsEnabled:       true,
		}),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_ValidConfigLatencyRouting_DefaultsForDetectionAndBanWindows_NotSet(t *testing.T) {
	config := `
    chains:
      - chainName: ethereum
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedConfig := Config{
		Chains: getCommonChainsConfig(nil),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_InvalidConfigLatencyRouting_InvalidRateInChainConfig(t *testing.T) {
	config := `
    chains:
      - chainName: ethereum
        routing:
          errors:
            rate: 1.25
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	_, err := parseConfig(configBytes)

	if err == nil {
		t.Errorf("Expected error parsing invalid YAML.")
	}
}

func TestParseConfig_InvalidConfigLatencyRouting_InvalidRateInGlobalConfig(t *testing.T) {
	config := `
    global:
      routing:
        errors:
          rate: -0.25

    chains:
      - chainName: ethereum
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	_, err := parseConfig(configBytes)

	if err == nil {
		t.Errorf("Expected error parsing invalid YAML.")
	}
}

func TestParseConfig_ValidConfigLatencyRouting_MethodLatencies_TopLevelLatencySpecifiedBothPerChainAndGlobal(t *testing.T) {
	config := `
    global:
      routing:
        latency:
          threshold: 1000ms
          methods:
            - method: getLogs
              threshold: 2000ms
            - method: eth_getStorageAt

    chains:
      - chainName: ethereum
        routing:
          latency:
            threshold: 4000ms
            methods:
              - method: getLogs
              - method: eth_getStorageAt
                threshold: 6000ms
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: RoutingConfig{
				DetectionWindow: NewDuration(DefaultDetectionWindow),
				BanWindow:       NewDuration(DefaultBanWindow),
				Latency: &LatencyConfig{
					MethodLatencyThresholds: map[string]time.Duration{
						"getLogs":          2000 * time.Millisecond,
						"eth_getStorageAt": 1000 * time.Millisecond, // Top-level latency default
					},
					Threshold: 1000 * time.Millisecond,
					Methods: []MethodConfig{
						{
							Name:      "getLogs",
							Threshold: 2000 * time.Millisecond,
						},
						{
							Name: "eth_getStorageAt",
						},
					},
				},
				Errors:        &ErrorsConfig{Rate: DefaultErrorRate},
				AlwaysRoute:   newBool(false),
				IsInitialized: true,
				IsEnabled:     true,
			},
		},

		Chains: getCommonChainsConfig(&RoutingConfig{
			DetectionWindow: NewDuration(DefaultDetectionWindow),
			BanWindow:       NewDuration(DefaultBanWindow),
			Latency: &LatencyConfig{
				MethodLatencyThresholds: map[string]time.Duration{
					"getLogs":          4000 * time.Millisecond, // Top-level latency default
					"eth_getStorageAt": 6000 * time.Millisecond,
				},
				Threshold: 4000 * time.Millisecond,
				Methods: []MethodConfig{
					{
						Name: "getLogs",
					},
					{
						Name:      "eth_getStorageAt",
						Threshold: 6000 * time.Millisecond,
					},
				},
			},
			Errors:        &ErrorsConfig{Rate: DefaultErrorRate},
			AlwaysRoute:   newBool(false),
			IsInitialized: true,
			IsEnabled:     true,
		}),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_ValidConfigLatencyRouting_MethodLatencies_TopLevelLatencyNotSpecifiedGlobalConfig(t *testing.T) {
	config := `
    global:
      routing:
        latency:
          methods:
            - method: getLogs
              threshold: 2000ms
            - method: eth_getStorageAt

    chains:
      - chainName: ethereum
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedLatencyConfig := LatencyConfig{
		MethodLatencyThresholds: map[string]time.Duration{
			"getLogs":          2000 * time.Millisecond,
			"eth_getStorageAt": DefaultMaxLatency, // Global default
		},
		Methods: []MethodConfig{
			{
				Name:      "getLogs",
				Threshold: 2000 * time.Millisecond,
			},
			{
				Name: "eth_getStorageAt",
			},
		},
		Threshold: DefaultMaxLatency,
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: RoutingConfig{
				DetectionWindow: NewDuration(DefaultDetectionWindow),
				BanWindow:       NewDuration(DefaultBanWindow),
				Latency:         &expectedLatencyConfig,
				Errors:          &ErrorsConfig{Rate: DefaultErrorRate},
				AlwaysRoute:     newBool(false),
				IsInitialized:   true,
				IsEnabled:       true,
			},
		},

		Chains: getCommonChainsConfig(&RoutingConfig{
			DetectionWindow: NewDuration(DefaultDetectionWindow),
			BanWindow:       NewDuration(DefaultBanWindow),
			Latency:         &expectedLatencyConfig,
			Errors:          &ErrorsConfig{Rate: DefaultErrorRate},
			AlwaysRoute:     newBool(false),
			IsInitialized:   true,
			IsEnabled:       true,
		}),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_ValidConfigLatencyRouting_MethodLatencies_TopLevelLatencySpecifiedInGlobalConfigOnly(t *testing.T) {
	config := `
    global:
      routing:
        latency:
          threshold: 1000ms
          methods:
            - method: getLogs
              threshold: 2000ms
            - method: eth_getStorageAt

    chains:
      - chainName: ethereum
        routing:
          latency:
            methods:
              - method: getLogs
              - method: eth_getStorageAt
                threshold: 6000ms
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: RoutingConfig{
				DetectionWindow: NewDuration(DefaultDetectionWindow),
				BanWindow:       NewDuration(DefaultBanWindow),
				Latency: &LatencyConfig{
					MethodLatencyThresholds: map[string]time.Duration{
						"getLogs":          2000 * time.Millisecond,
						"eth_getStorageAt": 1000 * time.Millisecond, // Top-level latency default
					},
					Threshold: 1000 * time.Millisecond,
					Methods: []MethodConfig{
						{
							Name:      "getLogs",
							Threshold: 2000 * time.Millisecond,
						},
						{
							Name: "eth_getStorageAt",
						},
					},
				},
				Errors:        &ErrorsConfig{Rate: DefaultErrorRate},
				AlwaysRoute:   newBool(false),
				IsInitialized: true,
				IsEnabled:     true,
			},
		},

		Chains: getCommonChainsConfig(&RoutingConfig{
			DetectionWindow: NewDuration(DefaultDetectionWindow),
			BanWindow:       NewDuration(DefaultBanWindow),
			Latency: &LatencyConfig{
				MethodLatencyThresholds: map[string]time.Duration{
					"getLogs":          2000 * time.Millisecond, // Top-level latency for method
					"eth_getStorageAt": 6000 * time.Millisecond,
				},
				Methods: []MethodConfig{
					{
						Name: "getLogs",
					},
					{
						Name:      "eth_getStorageAt",
						Threshold: 6000 * time.Millisecond,
					},
				},
			},
			Errors:        &ErrorsConfig{Rate: DefaultErrorRate},
			AlwaysRoute:   newBool(false),
			IsInitialized: true,
			IsEnabled:     true,
		}),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_ValidConfigLatencyRouting_MethodLatencies_TopLevelLatencyNotSpecifiedNeitherGlobalNotChain(t *testing.T) {
	config := `
    global:
      routing:
        latency:
          methods:
            - method: getLogs
              threshold: 2000ms
            - method: eth_getStorageAt
            - method: eth_chainId
              threshold: 20ms
            - method: eth_yetAnotherMethod
              threshold: 218ms
        errors:
          rate: 0.88
          httpCodes:
            - 5xx
            - 420
          jsonRpcCodes:
            - 32xxx
          errorStrings:
            - "internal server error"

    chains:
      - chainName: ethereum
        routing:
          latency:
            methods:
              - method: getLogs
              - method: eth_getStorageAt
                threshold: 6000ms
              - method: eth_doesSomethingImportant
              - method: eth_yetAnotherMethod
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedErrorsConfig := ErrorsConfig{
		Rate: 0.88,
		HTTPCodes: []string{
			"420",
			"5xx",
		},
		JSONRPCCodes: []string{
			"32xxx",
		},
		ErrorStrings: []string{
			"internal server error",
		},
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: RoutingConfig{
				DetectionWindow: NewDuration(DefaultDetectionWindow),
				BanWindow:       NewDuration(DefaultBanWindow),
				Latency: &LatencyConfig{
					MethodLatencyThresholds: map[string]time.Duration{
						"getLogs":              2000 * time.Millisecond,
						"eth_getStorageAt":     DefaultMaxLatency, // Top-level latency default
						"eth_chainId":          20 * time.Millisecond,
						"eth_yetAnotherMethod": 218 * time.Millisecond,
					},
					Methods: []MethodConfig{
						{
							Name:      "getLogs",
							Threshold: 2000 * time.Millisecond,
						},
						{
							Name: "eth_getStorageAt",
						},
						{
							Name:      "eth_chainId",
							Threshold: 20 * time.Millisecond,
						},
						{
							Name:      "eth_yetAnotherMethod",
							Threshold: 218 * time.Millisecond,
						},
					},
					Threshold: DefaultMaxLatency,
				},
				Errors:        &expectedErrorsConfig,
				AlwaysRoute:   newBool(false),
				IsInitialized: true,
				IsEnabled:     true,
			},
		},

		Chains: getCommonChainsConfig(&RoutingConfig{
			DetectionWindow: NewDuration(DefaultDetectionWindow),
			BanWindow:       NewDuration(DefaultBanWindow),
			Latency: &LatencyConfig{
				MethodLatencyThresholds: map[string]time.Duration{
					"getLogs":                    2000 * time.Millisecond, // Top-level latency for method
					"eth_getStorageAt":           6000 * time.Millisecond,
					"eth_doesSomethingImportant": DefaultMaxLatency,
					"eth_chainId":                20 * time.Millisecond, // Inherited from global latency config
					"eth_yetAnotherMethod":       218 * time.Millisecond,
				},
				Methods: []MethodConfig{
					{
						Name: "getLogs",
					},
					{
						Name:      "eth_getStorageAt",
						Threshold: 6000 * time.Millisecond,
					},
					{
						Name: "eth_doesSomethingImportant",
					},
					{
						Name: "eth_yetAnotherMethod",
					},
				},
			},
			Errors:        &expectedErrorsConfig,
			AlwaysRoute:   newBool(false),
			IsInitialized: true,
			IsEnabled:     true,
		}),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
}

func TestParseConfig_ValidConfigLatencyRouting_NoGlobalRoutingConfig_TwoChains_OnlyOneHasRoutingConfig(t *testing.T) {
	Assert := assert.New(t)

	config := `
    chains:
      - chainName: ethereum
        routing:
          latency:
            methods:
              - method: getLogs
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full

      - chainName: ethereum
        routing:
          latency:
            methods:
              - method: getLogs
                threshold: 210ms
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full

      - chainName: ethereum
        groups:
          - id: primary
            priority: 0
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            group: primary
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: RoutingConfig{
				DetectionWindow: NewDuration(DefaultDetectionWindow),
				BanWindow:       NewDuration(DefaultBanWindow),
				Latency: &LatencyConfig{
					MethodLatencyThresholds: map[string]time.Duration{},
				},
				Errors:        &ErrorsConfig{Rate: 0.25},
				AlwaysRoute:   newBool(false),
				IsInitialized: true,
				IsEnabled:     true,
			},
		},
		Chains: append(getCommonChainsConfig(&RoutingConfig{
			DetectionWindow: NewDuration(DefaultDetectionWindow),
			BanWindow:       NewDuration(DefaultBanWindow),
			Latency: &LatencyConfig{
				MethodLatencyThresholds: map[string]time.Duration{
					"getLogs": 10 * time.Second,
				},
				Methods: []MethodConfig{
					{
						Name: "getLogs",
					},
				},
			},
			Errors:        &ErrorsConfig{Rate: 0.25},
			AlwaysRoute:   newBool(false),
			IsInitialized: true,
			IsEnabled:     true,
		}), append(getCommonChainsConfig(&RoutingConfig{
			DetectionWindow: NewDuration(DefaultDetectionWindow),
			BanWindow:       NewDuration(DefaultBanWindow),
			Latency: &LatencyConfig{
				MethodLatencyThresholds: map[string]time.Duration{
					"getLogs": 210 * time.Millisecond,
				},
				Methods: []MethodConfig{
					{
						Name:      "getLogs",
						Threshold: 210 * time.Millisecond,
					},
				},
			},
			Errors:        &ErrorsConfig{Rate: 0.25},
			AlwaysRoute:   newBool(false),
			IsInitialized: true,
			IsEnabled:     true,
		}), getCommonChainsConfig(&RoutingConfig{})...)...),
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}

	Assert.True(parsedConfig.Chains[0].Routing.IsEnhancedRoutingControlDefined())
	Assert.True(parsedConfig.Chains[1].Routing.IsEnhancedRoutingControlDefined())
	Assert.False(parsedConfig.Chains[2].Routing.IsEnhancedRoutingControlDefined())
}

func TestParseConfig_InvalidYaml(t *testing.T) {
	config := `
    global:
      port: 8080

		invalid yaml

    upstreams:
      - id: alchemy-eth
        httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
        wsURL: "wss://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
      - id: ankr-polygon
        httpURL: "https://rpc.ankr.com/polygon"
        wsURL: "wss://rpc.ankr.com/polygon/ws/${ANKR_API_KEY}"
  `
	configBytes := []byte(config)

	_, err := parseConfig(configBytes)

	if err == nil {
		t.Errorf("Expected error parsing invalid YAML.")
	}
}

func TestCacheConfig_GetRedisAddresses(t *testing.T) {
	tests := []struct {
		name        string
		config      CacheConfig
		wantReader  string
		wantWriter  string
		description string
	}{
		{
			name: "legacy_config",
			config: CacheConfig{
				Redis: "localhost:6379",
			},
			wantReader:  "localhost:6379",
			wantWriter:  "localhost:6379",
			description: "Should use Redis field for both when only legacy config provided",
		},
		{
			name: "split_config",
			config: CacheConfig{
				RedisReader: "reader:6379",
				RedisWriter: "writer:6379",
			},
			wantReader:  "reader:6379",
			wantWriter:  "writer:6379",
			description: "Should use separate addresses when both reader and writer specified",
		},
		{
			name: "mixed_config",
			config: CacheConfig{
				Redis:       "legacy:6379",
				RedisReader: "reader:6379",
				RedisWriter: "writer:6379",
			},
			wantReader:  "reader:6379",
			wantWriter:  "writer:6379",
			description: "Should prefer new config over legacy when both present",
		},
		{
			name: "partial_config",
			config: CacheConfig{
				Redis:       "legacy:6379",
				RedisReader: "reader:6379",
			},
			wantReader:  "legacy:6379",
			wantWriter:  "legacy:6379",
			description: "Should fall back to legacy when new config is incomplete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer := tt.config.GetRedisAddresses()
			assert.Equal(t, tt.wantReader, reader, tt.description)
			assert.Equal(t, tt.wantWriter, writer, tt.description)
		})
	}
}

func TestChainCacheConfig_GetTTLForMethod(t *testing.T) {
	tests := []struct {
		config         ChainCacheConfig
		name           string
		method         string
		description    string
		expectedTTL    time.Duration
		expectedMinTTL time.Duration
	}{
		{
			name: "default_ttl_only",
			config: ChainCacheConfig{
				TTL:        5 * time.Minute,
				MethodTTLs: map[string]time.Duration{},
			},
			method:         "eth_getBalance",
			expectedTTL:    5 * time.Minute,
			expectedMinTTL: 5 * time.Minute,
			description:    "Should return default TTL when no method-specific TTL exists",
		},
		{
			name: "method_ttl_exists",
			config: ChainCacheConfig{
				TTL: 5 * time.Minute,
				MethodTTLs: map[string]time.Duration{
					"eth_getBalance": 10 * time.Minute,
				},
			},
			method:         "eth_getBalance",
			expectedTTL:    10 * time.Minute,
			expectedMinTTL: 5 * time.Minute,
			description:    "Should return method-specific TTL when it exists",
		},
		{
			name: "method_ttl_exists_query_non_existent_method",
			config: ChainCacheConfig{
				TTL: 5 * time.Minute,
				MethodTTLs: map[string]time.Duration{
					"eth_getBalance": 10 * time.Minute,
				},
			},
			method:         "eth_getLogs",
			expectedTTL:    5 * time.Minute,
			expectedMinTTL: 5 * time.Minute,
			description:    "Should return default TTL when method-specific TTL doesn't exists",
		},
		{
			name: "method_ttl_exists_but_not_default_query_non_existent_method",
			config: ChainCacheConfig{
				MethodTTLs: map[string]time.Duration{
					"eth_getBalance": 10 * time.Minute,
				},
			},
			method:         "eth_getLogs",
			expectedTTL:    0,
			expectedMinTTL: 10 * time.Minute,
			description:    "Should return default TTL (which is 0) when method-specific AND default TTL doesn't exist",
		},
		{
			name: "multiple_method_ttls_with_smaller_value",
			config: ChainCacheConfig{
				TTL: 5 * time.Minute,
				MethodTTLs: map[string]time.Duration{
					"eth_getBalance": 10 * time.Minute,
					"eth_getLogs":    3 * time.Minute,
				},
			},
			method:         "eth_getBalance",
			expectedTTL:    10 * time.Minute,
			expectedMinTTL: 3 * time.Minute,
			description:    "Should return method-specific TTL and minimum TTL should be the smallest value",
		},
		{
			name: "all_zero_values",
			config: ChainCacheConfig{
				TTL:        0,
				MethodTTLs: map[string]time.Duration{},
			},
			method:         "eth_getBalance",
			expectedTTL:    0,
			expectedMinTTL: 0,
			description:    "Should return 0 for both TTL and minimum TTL when all values are zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttl := tt.config.GetTTLForMethod(tt.method)
			assert.Equal(t, tt.expectedTTL, ttl, tt.description)

			minTTL := tt.config.GetMinimumTTL()
			assert.Equal(t, tt.expectedMinTTL, minTTL, "Minimum TTL should match expected value")
		})
	}
}

func TestParseConfig_ChainCacheConfig(t *testing.T) {
	config := `
    global:
      port: 8080

    chains:
      - chainName: ethereum
        cache:
          ttl: 5m
          methods:
            - method: eth_getBalance
              ttl: 10m
            - method: eth_getBlockByNumber
              ttl: 30s
        upstreams:
          - id: alchemy-eth
            httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            nodeType: full
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)
	assert.NoError(t, err)

	chainConfig := parsedConfig.Chains[0]

	// Verify default TTL
	assert.Equal(t, 5*time.Minute, chainConfig.Cache.TTL)

	// Verify method-specific TTLs
	assert.Equal(t, 10*time.Minute, chainConfig.Cache.MethodTTLs["eth_getBalance"])
	assert.Equal(t, 30*time.Second, chainConfig.Cache.MethodTTLs["eth_getBlockByNumber"])

	// Test GetTTLForMethod
	assert.Equal(t, 10*time.Minute, chainConfig.Cache.GetTTLForMethod("eth_getBalance"))
	assert.Equal(t, 30*time.Second, chainConfig.Cache.GetTTLForMethod("eth_getBlockByNumber"))
	assert.Equal(t, 5*time.Minute, chainConfig.Cache.GetTTLForMethod("eth_call")) // Not specified, should return default
}
