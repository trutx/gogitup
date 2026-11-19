package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gitutil "github.com/trutx/gogitup/internal/git"
)

// testGitEnv isolates tests from the developer's git configuration and from
// credential prompting.
func testGitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

// runGitIn runs git inside dir and fails the test if it does not succeed.
func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), string(out))
}

// initCommittedRepo turns dir into a repository holding a single commit.
func initCommittedRepo(t *testing.T, dir string) {
	t.Helper()
	runGitIn(t, dir, "init", "--initial-branch=main")
	runGitIn(t, dir, "config", "user.name", "Test User")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test content"), 0644))
	runGitIn(t, dir, "add", "test.txt")
	runGitIn(t, dir, "commit", "-m", "Initial commit")
}

// addBareRemote creates a bare repository holding dir's history and adds it as
// dir's origin remote.
func addBareRemote(t *testing.T, dir string) string {
	t.Helper()
	remoteDir := dir + "_remote.git"
	require.NoError(t, os.MkdirAll(remoteDir, 0755))
	runGitIn(t, remoteDir, "init", "--bare", "--initial-branch=main")
	runGitIn(t, dir, "remote", "add", "origin", remoteDir)
	runGitIn(t, dir, "push", "-u", "origin", "main")
	return remoteDir
}

func TestUpdateCommand_ReposFileAge(t *testing.T) {
	tests := []struct {
		name          string
		fileAge       time.Duration
		noScan        bool
		expectedError string
	}{
		{
			name:          "fresh_file_with_auto_scan",
			fileAge:       time.Hour * 24,
			expectedError: "no repositories found",
		},
		{
			name:          "old_file_with_auto_scan",
			fileAge:       time.Hour * 24 * 15,
			expectedError: "no repositories found",
		},
		{
			name:          "old_file_with_no_scan",
			fileAge:       time.Hour * 24 * 15,
			noScan:        true,
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

			// Create test config file pointing to empty tmpDir (no repos to find)
			emptyDir := filepath.Join(tmpDir, "empty")
			err = os.MkdirAll(emptyDir, 0755)
			require.NoError(t, err)
			configFile := filepath.Join(tmpDir, "config.yaml")
			err = os.WriteFile(configFile, []byte("directories: [\""+emptyDir+"\"]"), 0644)
			require.NoError(t, err)

			// Set up viper config
			viper.Reset()
			viper.Set("repos-file", reposFile)
			viper.Set("config", configFile)

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
			noScan = false
			threads = runtime.NumCPU()

			// Set command line arguments
			if tt.noScan {
				cmd.SetArgs([]string{"--no-scan"})
			}

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
		{
			name:          "no_scan_flag",
			args:          []string{"--no-scan"},
			expectedError: "no repositories found",
			checkThreads:  runtime.NumCPU(),
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

			// Create test config file pointing to tmpDir
			configFile := filepath.Join(tmpDir, "config.yaml")
			err = os.WriteFile(configFile, []byte("directories: [\""+tmpDir+"\"]"), 0644)
			require.NoError(t, err)

			// Set up test repositories if needed
			reposFile := filepath.Join(tmpDir, "repositories.json")
			if tt.setupRepos {
				// Create a test Git repository
				repoDir := filepath.Join(tmpDir, "repo")
				err := os.MkdirAll(repoDir, 0755)
				require.NoError(t, err)

				testGitEnv(t)
				initCommittedRepo(t, repoDir)
				addBareRemote(t, repoDir)

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
			noScan = false
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

func TestUpdateCommand_AutoScanConfig(t *testing.T) {
	// Create temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "gogitup-test-*")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create a git repo so scan has something to find
	repoDir := filepath.Join(tmpDir, "repo")
	err = os.MkdirAll(repoDir, 0755)
	require.NoError(t, err)

	testGitEnv(t)
	initCommittedRepo(t, repoDir)
	addBareRemote(t, repoDir)

	// Create config with auto_scan: false
	configFile := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configFile, []byte("directories:\n  - "+tmpDir+"\nauto_scan: false\n"), 0644)
	require.NoError(t, err)

	// Create empty repos file - auto-scan should NOT populate it
	reposFile := filepath.Join(tmpDir, "repositories.json")
	err = os.WriteFile(reposFile, []byte("[]"), 0644)
	require.NoError(t, err)

	// Reset viper and set config
	viper.Reset()
	viper.Set("repos-file", reposFile)
	viper.Set("config", configFile)

	// Create a new command instance
	cmd := &cobra.Command{Use: "update"}
	cmd.RunE = updateCmd.RunE
	cmd.PreRun = updateCmd.PreRun
	cmd.Flags().AddFlagSet(updateCmd.Flags())
	cmd.PersistentFlags().AddFlagSet(rootCmd.PersistentFlags())

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
	verbose = false
	noScan = false
	threads = runtime.NumCPU()

	// Execute update command - should skip scan due to auto_scan: false
	err = cmd.Execute()
	assert.ErrorContains(t, err, "no repositories found")
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
