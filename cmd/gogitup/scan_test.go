package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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

func setupTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "scan"}
	cmd.RunE = scanCmd.RunE
	cmd.PreRun = scanCmd.PreRun

	// Add all flags from the original command
	cmd.Flags().String("config", "", "config file")
	cmd.Flags().String("repos-file", "", "repositories file")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	return cmd
}

func TestScanCommand_Flags(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		setupConfig    bool
		expectedError  string
		checkVerbose   bool
		expectedOutput string
	}{
		{
			name:          "no_flags",
			args:          []string{},
			setupConfig:   true,
			expectedError: "",
		},
		{
			name:          "invalid_config_file",
			args:          []string{"--config", "/nonexistent/config.yaml"},
			setupConfig:   false,
			expectedError: "failed to load config",
		},
		{
			name:          "verbose_flag",
			args:          []string{"--verbose"},
			setupConfig:   true,
			checkVerbose:  true,
			expectedError: "",
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

			// Reset viper
			viper.Reset()

			// Create test config file
			configFile := filepath.Join(tmpDir, "config.yaml")
			if tt.setupConfig {
				err = os.WriteFile(configFile, []byte("directories: [\".\"]"), 0644)
				require.NoError(t, err)

				// Set up viper with the config file
				viper.SetConfigFile(configFile)
				err = viper.ReadInConfig()
				require.NoError(t, err)
			}

			// Create repositories file
			reposFile := filepath.Join(tmpDir, "repositories.json")
			err = os.WriteFile(reposFile, []byte("[]"), 0644)
			require.NoError(t, err)
			viper.Set("repos-file", reposFile)

			// Create a new command instance for each test
			cmd := setupTestCommand()

			// Reset flags and verbose mode before test
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
			})
			verbose = false

			// Set command line arguments
			cmd.SetArgs(tt.args)

			// Execute scan command
			err = cmd.Execute()
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Check if verbose mode was enabled
			if tt.checkVerbose {
				assert.True(t, verbose, "verbose mode should be enabled when using --verbose flag")
			}
		})
	}
}

func TestScanCommand_RepositoryDiscovery(t *testing.T) {
	tests := []struct {
		name              string
		setupDirs         []string
		setupRepos        bool
		expectedRepoCount int
		expectedError     string
	}{
		{
			name:              "single_repo",
			setupDirs:         []string{"repo1"},
			setupRepos:        true,
			expectedRepoCount: 1,
		},
		{
			name:              "multiple_repos",
			setupDirs:         []string{"repo1", "repo2", "repo3"},
			setupRepos:        true,
			expectedRepoCount: 3,
		},
		{
			name:              "no_repos",
			setupDirs:         []string{"empty1", "empty2"},
			setupRepos:        false,
			expectedRepoCount: 0,
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

			// Create test directories and repositories
			var repoDirs []string
			for _, dir := range tt.setupDirs {
				repoDir := filepath.Join(tmpDir, dir)
				err := os.MkdirAll(repoDir, 0755)
				require.NoError(t, err)
				repoDirs = append(repoDirs, repoDir)

				if tt.setupRepos {
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
					remoteDir := filepath.Join(tmpDir, dir+"_remote.git")
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
				}
			}

			// Create test config file
			configFile := filepath.Join(tmpDir, "config.yaml")
			configContent := "directories:\n"
			for _, dir := range repoDirs {
				configContent += "  - " + dir + "\n"
			}
			err = os.WriteFile(configFile, []byte(configContent), 0644)
			require.NoError(t, err)

			// Create repositories file
			reposFile := filepath.Join(tmpDir, "repositories.json")
			err = os.WriteFile(reposFile, []byte("[]"), 0644)
			require.NoError(t, err)

			// Reset viper and set config
			viper.Reset()
			viper.Set("config", configFile)
			viper.Set("repos-file", reposFile)

			// Create a new command instance
			cmd := setupTestCommand()

			// Execute scan command
			err = cmd.Execute()
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)

				// Verify repositories file content
				data, err := os.ReadFile(reposFile)
				require.NoError(t, err)

				var repos []gitutil.Repository
				err = json.Unmarshal(data, &repos)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedRepoCount, len(repos))
			}
		})
	}
}

func TestScanCommand_Output(t *testing.T) {
	tests := []struct {
		name           string
		setupRepos     bool
		verbose        bool
		expectedOutput []string
	}{
		{
			name:       "normal_output",
			setupRepos: true,
			verbose:    false,
			expectedOutput: []string{
				"Found 1 repositories",
				"Results saved to:",
			},
		},
		{
			name:       "verbose_output",
			setupRepos: true,
			verbose:    true,
			expectedOutput: []string{
				"Found 1 repositories",
				"Repository list:",
				"Results saved to:",
			},
		},
		{
			name:       "no_repos",
			setupRepos: false,
			verbose:    true,
			expectedOutput: []string{
				"Found 0 repositories",
				"Repository list:",
				"Results saved to:",
			},
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

			// Create test repository if needed
			if tt.setupRepos {
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
			}

			// Create test config file
			configFile := filepath.Join(tmpDir, "config.yaml")
			err = os.WriteFile(configFile, []byte("directories: [\""+tmpDir+"\"]"), 0644)
			require.NoError(t, err)

			// Create repositories file
			reposFile := filepath.Join(tmpDir, "repositories.json")
			err = os.WriteFile(reposFile, []byte("[]"), 0644)
			require.NoError(t, err)

			// Reset viper and set config
			viper.Reset()
			viper.Set("config", configFile)
			viper.Set("repos-file", reposFile)

			// Create a new command instance
			cmd := setupTestCommand()

			// Set verbose mode
			if tt.verbose {
				cmd.SetArgs([]string{"--verbose"})
			}

			// Capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Execute scan command
			err = cmd.Execute()
			require.NoError(t, err)

			// Restore stdout
			err = w.Close()
			require.NoError(t, err)
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			require.NoError(t, err)
			output := buf.String()

			// Check expected output strings
			for _, expectedStr := range tt.expectedOutput {
				assert.Contains(t, output, expectedStr)
			}
		})
	}
}
