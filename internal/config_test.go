package internal

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
	} {
		configBytes := []byte(testCase.config)
		_, err := parseConfig(configBytes)
		assert.NotNil(t, err)
	}
}

func newBool(b bool) *bool {
	return &b
}

func TestParseConfig_ValidConfig(t *testing.T) {
	config := `
    global:
      port: 8080

    upstreams:
      - id: alchemy-eth
        httpURL: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
        wsURL: "wss://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
        healthCheck:
          useWsForBlockHeight: true
      - id: ankr-polygon
        httpURL: "https://rpc.ankr.com/polygon"
        wsURL: "wss://rpc.ankr.com/polygon/ws/${ANKR_API_KEY}"
  `
	configBytes := []byte(config)

	parsedConfig, err := parseConfig(configBytes)

	if err != nil {
		t.Errorf("parseConfig returned error: %v", err)
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
			},
			{
				ID:      "ankr-polygon",
				HTTPURL: "https://rpc.ankr.com/polygon",
				WSURL:   "wss://rpc.ankr.com/polygon/ws/${ANKR_API_KEY}",
				HealthCheckConfig: HealthCheckConfig{
					UseWSForBlockHeight: nil,
				},
			},
		},
		Global: GlobalConfig{
			Port: 8080,
		},
	}

	if !reflect.DeepEqual(parsedConfig, expectedConfig) {
		t.Errorf("parseConfig returned unexpected config: %v", parsedConfig)
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
