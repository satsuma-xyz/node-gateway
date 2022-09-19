package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfig_InvalidConfigs(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		config string
	}{
		{
			name: "Upstream config without httpURL.",
			config: `
            global:
              port: 8080

            upstreams:
              - id: alchemy-eth
                wsURL: "wss://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                healthCheck:
                  useWsForBlockHeight: true
            `,
		},
		{
			name: "UpstreamConfig without wssURL when useWsForBlockHeight: true.",
			config: `
            global:
              port: 8080

            upstreams:
              - id: alchemy-eth
                httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                healthCheck:
                  useWsForBlockHeight: true
            `,
		},
		{
			name: "Groups with same priority.",
			config: `
            global:
              port: 8080

            upstreams:
              - id: alchemy-eth
                httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                group: primary
            
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

            upstreams:
              - id: alchemy-eth
                httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
            
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

            upstreams:
              - id: alchemy-eth
                httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
                group: something-that-doesnt-exist
            
            groups:
              - id: primary
                priority: 0
            `,
		},
	} {
		configBytes := []byte(testCase.config)
		_, err := parseConfig(configBytes)
		assert.NotNil(t, err)

		// To prevent catching formatting errors, that's not what we're checking for in this test.
		assert.NotContains(t, err.Error(), "found character that cannot start any token", testCase.config)
		assert.NotContains(t, err.Error(), "found a tab character that violates indentation", testCase.config)
	}
}

func newBool(b bool) *bool {
	return &b
}

func TestParseConfig_ValidConfig(t *testing.T) {
	config := `
    global:
      port: 8080

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
      - id: ankr-polygon
        httpURL: "https://rpc.ankr.com/polygon"
        wsURL: "wss://rpc.ankr.com/polygon/ws/${ANKR_API_KEY}"
        group: fallback
        nodeType: archive
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("ParseConfig returned error: %v.", err)
	}

	expectedConfig := Config{
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
			},
		},
		Global: GlobalConfig{
			Port: 8080,
		},
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
	}

	if !reflect.DeepEqual(parsedConfig, expectedConfig) {
		t.Errorf("ParseConfig returned unexpected config: %v.", parsedConfig)
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
