package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Directories []string `mapstructure:"directories"`
}

// LoadConfig loads the configuration from the config file
func LoadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Get config file path from viper
	configFile := viper.GetString("config")

	// If config file starts with ~, replace with home directory
	if len(configFile) >= 2 && configFile[:2] == "~/" {
		configFile = filepath.Join(home, configFile[2:])
	}

	// Set the config file path explicitly
	viper.SetConfigFile(configFile)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if len(config.Directories) == 0 {
		return nil, fmt.Errorf("no directories configured")
	}

	// Expand any environment variables or ~ in directory paths
	for i, dir := range config.Directories {
		if dir == "~" {
			config.Directories[i] = home
		} else if len(dir) >= 2 && dir[:2] == "~/" {
			config.Directories[i] = filepath.Join(home, dir[2:])
		}
		config.Directories[i] = os.ExpandEnv(config.Directories[i])
	}

	return &config, nil
}
