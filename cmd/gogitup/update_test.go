package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gitutil "github.com/trutx/gogitup/internal/git"
)

func TestPromptForScan(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"answer_yes", "y\n", true},
		{"answer_YES_(uppercase)", "Y\n", true},
		{"answer_no", "n\n", false},
		{"answer_NO_(uppercase)", "N\n", false},
		{"empty_answer_(default_yes)", "\n", true},
		{"invalid_answer_(default_yes)", "invalid\n", true},
		{"reader_error", "", true}, // Test error case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file for input simulation
			tmpfile, err := os.CreateTemp("", "test")
			require.NoError(t, err)
			defer func() {
				err := os.Remove(tmpfile.Name())
				if err != nil {
					t.Errorf("Failed to remove temp file: %v", err)
				}
			}()

			// Write test input to file
			_, err = tmpfile.Write([]byte(tt.input))
			require.NoError(t, err)
			err = tmpfile.Close()
			require.NoError(t, err)

			// Redirect stdin to our test file
			oldStdin := os.Stdin
			tmpfile, err = os.Open(tmpfile.Name())
			require.NoError(t, err)
			os.Stdin = tmpfile
			defer func() {
				os.Stdin = oldStdin
				err := tmpfile.Close()
				if err != nil {
					t.Errorf("Failed to close temp file: %v", err)
				}
			}()

			result := promptForScan()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateCommand_ReposFileAge(t *testing.T) {
	tests := []struct {
		name           string
		fileAge        time.Duration
		scanResponse   string
		expectedError  string
		expectedOutput string
	}{
		{
			name:          "fresh_file",
			fileAge:       time.Hour * 24,
			expectedError: "no repositories found",
		},
		{
			name:          "old_file, answer_no",
			fileAge:       time.Hour * 24 * 15,
			scanResponse:  "n\n",
			expectedError: "no repositories found",
		},
		{
			name:          "old_file, answer_yes",
			fileAge:       time.Hour * 24 * 15,
			scanResponse:  "y\n",
			expectedError: "no repositories found",
		},
		{
			name:          "old_file, empty_answer_(default_yes)",
			fileAge:       time.Hour * 24 * 15,
			scanResponse:  "\n",
			expectedError: "no repositories found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test files
			tmpDir, err := os.MkdirTemp("", "gogitup-test-*")
			require.NoError(t, err)
			defer func() {
				err := os.RemoveAll(tmpDir)
				if err != nil {
					t.Errorf("Failed to remove temp directory: %v", err)
				}
			}()

			// Create temporary repositories file
			reposFile := filepath.Join(tmpDir, "repositories.json")
			err = os.WriteFile(reposFile, []byte("[]"), 0644)
			require.NoError(t, err)

			// Set file modification time
			modTime := time.Now().Add(-tt.fileAge)
			err = os.Chtimes(reposFile, modTime, modTime)
			require.NoError(t, err)

			// Create test config file
			configFile := filepath.Join(tmpDir, "config.yaml")
			err = os.WriteFile(configFile, []byte("directories: [\".\"]"), 0644)
			require.NoError(t, err)

			// Set up viper config
			viper.Reset()
			viper.Set("repos-file", reposFile)
			viper.Set("config", configFile)

			// Create a temporary file for input simulation if needed
			if tt.scanResponse != "" {
				tmpfile, err := os.CreateTemp("", "test")
				require.NoError(t, err)
				defer func() {
					err := os.Remove(tmpfile.Name())
					if err != nil {
						t.Errorf("Failed to remove temp file: %v", err)
					}
				}()

				_, err = tmpfile.Write([]byte(tt.scanResponse))
				require.NoError(t, err)
				err = tmpfile.Close()
				require.NoError(t, err)

				// Redirect stdin to our test file
				oldStdin := os.Stdin
				tmpfile, err = os.Open(tmpfile.Name())
				require.NoError(t, err)
				os.Stdin = tmpfile
				defer func() {
					os.Stdin = oldStdin
					err := tmpfile.Close()
					if err != nil {
						t.Errorf("Failed to close temp file: %v", err)
					}
				}()
			}

			// Reset flags before test
			updateCmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
			})

			// Create a new command instance for each test
			cmd := &cobra.Command{Use: "update"}
			cmd.RunE = updateCmd.RunE
			cmd.PreRun = updateCmd.PreRun
			cmd.Flags().AddFlagSet(updateCmd.Flags())
			cmd.PersistentFlags().AddFlagSet(rootCmd.PersistentFlags())

			// Reset flags and verbose mode before test
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
			})
			verbose = false
			threads = runtime.NumCPU() // Reset threads to default

			// Execute update command
			err = cmd.Execute()
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateCommand_Flags(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		setupRepos     bool
		expectedError  string
		checkVerbose   bool
		checkThreads   int
		expectedOutput string
	}{
		{
			name:          "no_flags",
			args:          []string{},
			expectedError: "no repositories found",
			checkThreads:  runtime.NumCPU(), // Default should be NumCPU
		},
		{
			name:          "invalid_threads",
			args:          []string{"--threads", "-1"},
			expectedError: "no repositories found",
			checkThreads:  1, // Should be clamped to minimum of 1
		},
		{
			name:          "valid_threads",
			args:          []string{"--threads", "4"},
			expectedError: "no repositories found",
			checkThreads:  4,
		},
		{
			name:          "threads_exceeds_repos",
			args:          []string{"--threads", "100"},
			setupRepos:    true,
			expectedError: "",
			checkThreads:  1, // Should be clamped to number of repos (1 in test)
		},
		{
			name:          "stat_flag_enables_verbose",
			args:          []string{"--stat"},
			setupRepos:    true,
			checkVerbose:  true,
			expectedError: "",
			checkThreads:  runtime.NumCPU(), // Default should be NumCPU
		},
		{
			name:          "invalid_config_file",
			args:          []string{"--config", "/nonexistent/config.yaml"},
			expectedError: "no repositories found",
			checkThreads:  runtime.NumCPU(), // Default should be NumCPU
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test files
			tmpDir, err := os.MkdirTemp("", "gogitup-test-*")
			require.NoError(t, err)
			defer func() {
				err := os.RemoveAll(tmpDir)
				if err != nil {
					t.Errorf("Failed to remove temp directory: %v", err)
				}
			}()

			// Create test config file
			configFile := filepath.Join(tmpDir, "config.yaml")
			err = os.WriteFile(configFile, []byte("directories: [\".\"]"), 0644)
			require.NoError(t, err)

			// Set up test repositories if needed
			reposFile := filepath.Join(tmpDir, "repositories.json")
			if tt.setupRepos {
				// Create a test Git repository
				repoDir := filepath.Join(tmpDir, "repo")
				err := os.MkdirAll(repoDir, 0755)
				require.NoError(t, err)

				repo, err := git.PlainInit(repoDir, false)
				require.NoError(t, err)

				// Create a test file and commit it
				testFile := filepath.Join(repoDir, "test.txt")
				err = os.WriteFile(testFile, []byte("test content"), 0644)
				require.NoError(t, err)

				w, err := repo.Worktree()
				require.NoError(t, err)

				_, err = w.Add("test.txt")
				require.NoError(t, err)

				_, err = w.Commit("Initial commit", &git.CommitOptions{
					Author: &object.Signature{
						Name:  "Test User",
						Email: "test@example.com",
						When:  time.Now(),
					},
				})
				require.NoError(t, err)

				// Create a bare clone to serve as the remote
				remoteDir := filepath.Join(tmpDir, "remote.git")
				_, err = git.PlainClone(remoteDir, true, &git.CloneOptions{
					URL: repoDir,
				})
				require.NoError(t, err)

				// Add the remote to the original repository
				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{remoteDir},
				})
				require.NoError(t, err)

				// Save repository info
				repoInfo := gitutil.Repository{
					Path:        repoDir,
					HasUpstream: false,
					LastScanned: time.Now(),
				}

				// Save repository info to file
				data, err := json.Marshal([]gitutil.Repository{repoInfo})
				require.NoError(t, err)
				err = os.WriteFile(reposFile, data, 0644)
				require.NoError(t, err)
			} else {
				err = os.WriteFile(reposFile, []byte("[]"), 0644)
				require.NoError(t, err)
			}

			// Reset viper and set config
			viper.Reset()
			viper.Set("repos-file", reposFile)
			viper.Set("config", configFile)

			// Create a new command instance for each test
			cmd := &cobra.Command{Use: "update"}
			cmd.RunE = updateCmd.RunE
			cmd.PreRun = updateCmd.PreRun
			cmd.Flags().AddFlagSet(updateCmd.Flags())
			cmd.PersistentFlags().AddFlagSet(rootCmd.PersistentFlags())

			// Reset flags and verbose mode before test
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
			})
			verbose = false
			threads = runtime.NumCPU() // Reset threads to default

			// Set command line arguments
			cmd.SetArgs(tt.args)

			// Execute update command
			err = cmd.Execute()
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Check if verbose mode was enabled by stat flag
			if tt.checkVerbose {
				assert.True(t, verbose, "verbose mode should be enabled when using --stat flag")
			}

			// Check if threads value is correct
			if tt.checkThreads > 0 {
				assert.Equal(t, tt.checkThreads, threads, "threads value should match expected value")
			}
		})
	}
}

func TestRunScan(t *testing.T) {
	tests := []struct {
		name          string
		setupConfig   func(string) error
		expectedError string
	}{
		{
			name: "valid_config",
			setupConfig: func(configFile string) error {
				err := os.WriteFile(configFile, []byte("directories: [\".\"]"), 0644)
				if err != nil {
					return err
				}
				viper.Set("config", configFile)
				viper.Set("directories", []string{"."})
				return nil
			},
			expectedError: "",
		},
		{
			name: "invalid_config",
			setupConfig: func(configFile string) error {
				err := os.WriteFile(configFile, []byte("invalid: yaml: content"), 0644)
				if err != nil {
					return err
				}
				viper.Set("config", configFile)
				return nil
			},
			expectedError: "failed to load config",
		},
		{
			name: "empty_directories",
			setupConfig: func(configFile string) error {
				err := os.WriteFile(configFile, []byte("directories: []"), 0644)
				if err != nil {
					return err
				}
				viper.Set("config", configFile)
				viper.Set("directories", []string{})
				return nil
			},
			expectedError: "failed to load config: no directories configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test files
			tmpDir, err := os.MkdirTemp("", "gogitup-test-*")
			require.NoError(t, err)
			defer func() {
				err := os.RemoveAll(tmpDir)
				if err != nil {
					t.Errorf("Failed to remove temp directory: %v", err)
				}
			}()

			// Create test config file
			configFile := filepath.Join(tmpDir, "config.yaml")

			// Reset viper config before each test
			viper.Reset()

			// Set up test configuration
			err = tt.setupConfig(configFile)
			require.NoError(t, err)

			// Run scan
			err = runScan()
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
