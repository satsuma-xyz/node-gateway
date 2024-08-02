package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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

func TestParseConfig_ValidGlobalConfigLatencyRouting(t *testing.T) {
	config := `
    global:
      routing:
        detectionWindow: 1m
        banWindow: 5m
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

	expectedConfig := Config{
		Global: GlobalConfig{
			Routing: RoutingConfig{
				DetectionWindow: time.Minute,
				BanWindow:       5 * time.Minute,
				Errors: ErrorsConfig{
					Rate: 0.25,
					HTTPCodes: []string{
						"5xx",
						"420",
					},
					JSONRPCCodes: []string{
						"32xxx",
					},
					ErrorStrings: []string{
						"internal server error",
					},
				},
				Latency: LatencyConfig{
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
				},
				AlwaysRoute: newBool(true),
			},
		},
		Chains: []SingleChainConfig{{
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
		}},
	}

	if diff := cmp.Diff(expectedConfig, parsedConfig); diff != "" {
		t.Errorf("ParseConfig returned unexpected config - diff:\n%s", diff)
	}
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
