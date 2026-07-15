package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "gogitup-test-*")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Get user home directory for path expansion testing
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Create test config files
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := []byte(`
directories:
  - /path/to/repos1
  - /path/to/repos2
  - ~/code/projects
`)
	autoScanFalseConfig := filepath.Join(tmpDir, "autoscan_false.yaml")
	falseVal := false
	err = os.WriteFile(autoScanFalseConfig, []byte("directories:\n  - /path/to/repos1\nauto_scan: false\n"), 0644)
	require.NoError(t, err)

	autoScanTrueConfig := filepath.Join(tmpDir, "autoscan_true.yaml")
	trueVal := true
	err = os.WriteFile(autoScanTrueConfig, []byte("directories:\n  - /path/to/repos1\nauto_scan: true\n"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(configFile, configContent, 0644)
	require.NoError(t, err)

	tests := []struct {
		name         string
		setupConfig  func()
		wantDirs     []string
		wantAutoScan *bool
		wantErr      bool
		wantErrMatch string
	}{
		{
			name: "valid config file",
			setupConfig: func() {
				viper.Reset()
				viper.SetConfigFile(configFile)
			},
			wantDirs: []string{
				"/path/to/repos1",
				"/path/to/repos2",
				filepath.Join(home, "code/projects"),
			},
			wantAutoScan: nil,
			wantErr:      false,
		},
		{
			name: "auto_scan false",
			setupConfig: func() {
				viper.Reset()
				viper.SetConfigFile(autoScanFalseConfig)
			},
			wantDirs:     []string{"/path/to/repos1"},
			wantAutoScan: &falseVal,
			wantErr:      false,
		},
		{
			name: "auto_scan true",
			setupConfig: func() {
				viper.Reset()
				viper.SetConfigFile(autoScanTrueConfig)
			},
			wantDirs:     []string{"/path/to/repos1"},
			wantAutoScan: &trueVal,
			wantErr:      false,
		},
		{
			name: "missing config file",
			setupConfig: func() {
				viper.Reset()
				viper.SetConfigFile(filepath.Join(tmpDir, "nonexistent.yaml"))
			},
			wantErr:      true,
			wantErrMatch: "failed to read config file",
		},
		{
			name: "invalid config file",
			setupConfig: func() {
				viper.Reset()
				invalidConfig := filepath.Join(tmpDir, "invalid.yaml")
				err := os.WriteFile(invalidConfig, []byte("invalid: [yaml"), 0644)
				require.NoError(t, err)
				viper.SetConfigFile(invalidConfig)
			},
			wantErr:      true,
			wantErrMatch: "failed to read config file",
		},
		{
			name: "empty directories list",
			setupConfig: func() {
				viper.Reset()
				emptyConfig := filepath.Join(tmpDir, "empty.yaml")
				err := os.WriteFile(emptyConfig, []byte("directories: []"), 0644)
				require.NoError(t, err)
				viper.SetConfigFile(emptyConfig)
			},
			wantErr:      true,
			wantErrMatch: "no directories configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupConfig()

			cfg, err := LoadConfig()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMatch != "" {
					assert.Contains(t, err.Error(), tt.wantErrMatch)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantDirs, cfg.Directories)
				if tt.wantAutoScan == nil {
					assert.Nil(t, cfg.AutoScan)
				} else {
					require.NotNil(t, cfg.AutoScan)
					assert.Equal(t, *tt.wantAutoScan, *cfg.AutoScan)
				}
			}
		})
	}
}
