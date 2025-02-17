package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type CommonConfig struct {
	PollingInterval int    `yaml:"polling_interval"`
	region          string `yaml:"region"`
}
type Environment struct {
	CommonConfig `yaml:",inline"`
	ThingName    string `yaml:"thing_name"`
	IoTEndpoint  string `yaml:"iot_endpoint"`
	CertPath     string `yaml:"cert_path"`
	KeyPath      string `yaml:"key_path"`
	RootCAPath   string `yaml:"root_ca_path"`
}
type Config struct {
	Common       CommonConfig           `yaml:"common"`
	Environments map[string]Environment `yaml:"environments"`
	CurrentEnv   string                 `yaml:"current_environment"`
}

func LoadConfig(configPath string) (*Config, error) {
	file, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(file, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	return &cfg, nil
}
func (c *CommonConfig) Merge(other CommonConfig) {
	if other.PollingInterval != 0 {
		c.PollingInterval = other.PollingInterval
	}
	// Add merging for other common fields here
}
func (c *Config) GetCurrentEnvironment(overrideEnv string) (*Environment, error) {
	envName := c.CurrentEnv
	if overrideEnv != "" {
		envName = overrideEnv
	}
	env, exists := c.Environments[envName]
	if !exists {
		return nil, fmt.Errorf("environment %s does not exist", envName)
	}
	merged := c.Common
	merged.Merge(env.CommonConfig)

	mergedEnv := Environment{
		CommonConfig: merged,
		ThingName:    env.ThingName,
		IoTEndpoint:  env.IoTEndpoint,
		CertPath:     env.CertPath,
		KeyPath:      env.KeyPath,
		RootCAPath:   env.RootCAPath,
	}

	return &mergedEnv, nil
}

func (e *Environment) Validate() error {
	required := map[string]string{
		"thing_name":   e.ThingName,
		"iot_endpoint": e.IoTEndpoint,
		"cert_path":    e.CertPath,
		"key_path":     e.KeyPath,
		"root_ca_path": e.RootCAPath,
	}

	for name, value := range required {
		if value == "" {
			return fmt.Errorf("%s must be set", name)
		}
	}

	files := []string{e.CertPath, e.KeyPath, e.RootCAPath}
	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return fmt.Errorf("certificate file %s does not exist", file)
		}
	}

	return nil
}
