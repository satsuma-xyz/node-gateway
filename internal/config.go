package internal

import (
	"os"

	"gopkg.in/yaml.v3"
)

type UpstreamConfig struct {
	ID      string `yaml:"id"`
	Chain   string `yaml:"chain"`
	HTTPURL string `yaml:"httpURL"`
	WSURL   string `yaml:"wsURL"`
}

type GlobalConfig struct {
	Port int `yaml:"port"`
}

type Config struct {
	Upstreams []UpstreamConfig
	Global    GlobalConfig
}

func LoadConfig(configFilePath string) (config Config, err error) {
	configBytes, err := os.ReadFile(configFilePath)

	if err != nil {
		return Config{}, err
	}

	return parseConfig(configBytes)
}

func parseConfig(configBytes []byte) (config Config, err error) {
	config = Config{}
	err = yaml.Unmarshal(configBytes, &config)

	return config, err
}
