package git

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGitEnv isolates tests from the developer's git configuration and from
// credential prompting.
func testGitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

// runGitCmd runs git in dir and fails the test if it does not succeed.
func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), string(out))
	return string(out)
}

// configureUser gives the repository a local commit identity.
func configureUser(t *testing.T, dir string) {
	t.Helper()
	runGitCmd(t, dir, "config", "user.name", "Test User")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
}

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	testGitEnv(t)

	baseDir, err := os.MkdirTemp("", "gogitup-test-*")
	require.NoError(t, err)

	originDir := filepath.Join(baseDir, "origin")
	localDir := filepath.Join(baseDir, "local")
	require.NoError(t, os.MkdirAll(originDir, 0755))
	require.NoError(t, os.MkdirAll(localDir, 0755))
	t.Logf("Created test directories: origin=%s, local=%s", originDir, localDir)

	// Initialize a bare repository for origin
	runGitCmd(t, originDir, "init", "--bare", "--initial-branch=main")
	t.Log("Initialized bare origin repository")

	// Initialize the local repository with a single commit
	runGitCmd(t, localDir, "init", "--initial-branch=main")
	configureUser(t, localDir)
	t.Log("Initialized local repository")

	err = os.WriteFile(filepath.Join(localDir, "test.txt"), []byte("test content"), 0644)
	require.NoError(t, err)
	runGitCmd(t, localDir, "add", "test.txt")
	runGitCmd(t, localDir, "commit", "-m", "Initial commit")
	t.Log("Created initial commit")

	runGitCmd(t, localDir, "remote", "add", "origin", originDir)
	runGitCmd(t, localDir, "push", "-u", "origin", "main")
	t.Log("Pushed to origin")

	cleanup := func() {
		if err := os.RemoveAll(baseDir); err != nil {
			t.Errorf("Failed to remove test directory: %v", err)
		}
		t.Log("Cleaned up test directories")
	}

	return localDir, cleanup
}

func setupTestRepoWithRemotes(t *testing.T) (string, string, string, func()) {
	t.Helper()
	testGitEnv(t)

	baseDir, err := os.MkdirTemp("", "gogitup-test-*")
	require.NoError(t, err)

	originDir := filepath.Join(baseDir, "origin")
	upstreamDir := filepath.Join(baseDir, "upstream")
	localDir := filepath.Join(baseDir, "local")
	for _, dir := range []string{originDir, upstreamDir, localDir} {
		require.NoError(t, os.MkdirAll(dir, 0755))
	}
	t.Logf("Created test directories: origin=%s, upstream=%s, local=%s", originDir, upstreamDir, localDir)

	// Initialize upstream repository with a single commit
	runGitCmd(t, upstreamDir, "init", "--initial-branch=main")
	configureUser(t, upstreamDir)
	t.Log("Initialized upstream repository")

	err = os.WriteFile(filepath.Join(upstreamDir, "test.txt"), []byte("test content"), 0644)
	require.NoError(t, err)
	runGitCmd(t, upstreamDir, "add", "test.txt")
	runGitCmd(t, upstreamDir, "commit", "-m", "Initial commit")
	t.Log("Created initial commit in upstream")

	// Initialize origin repository as bare
	runGitCmd(t, originDir, "init", "--bare", "--initial-branch=main")
	t.Log("Initialized bare origin repository")

	// Clone upstream to local, then turn it into a fork of origin
	runGitCmd(t, localDir, "clone", upstreamDir, ".")
	configureUser(t, localDir)
	t.Log("Cloned upstream to local")

	runGitCmd(t, localDir, "remote", "rename", "origin", "upstream")
	runGitCmd(t, localDir, "remote", "add", "origin", originDir)
	runGitCmd(t, localDir, "push", "-u", "origin", "main")
	t.Log("Pushed to origin")

	// Create a new commit in upstream so the fork is behind it
	err = os.WriteFile(filepath.Join(upstreamDir, "upstream.txt"), []byte("upstream content"), 0644)
	require.NoError(t, err)
	runGitCmd(t, upstreamDir, "add", "upstream.txt")
	runGitCmd(t, upstreamDir, "commit", "-m", "Upstream commit")
	t.Log("Created new commit in upstream")

	cleanup := func() {
		if err := os.RemoveAll(baseDir); err != nil {
			t.Errorf("Failed to remove test directory: %v", err)
		}
		t.Log("Cleaned up test directories")
	}

	return localDir, originDir, upstreamDir, cleanup
}

func TestFindRepositories(t *testing.T) {
	// Create test repositories
	repo1Dir, cleanup1 := setupTestRepo(t)
	defer cleanup1()
	repo2Dir, cleanup2 := setupTestRepo(t)
	defer cleanup2()

	// Create a non-repository directory
	nonRepoDir, err := os.MkdirTemp("", "non-repo-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := os.RemoveAll(nonRepoDir); err != nil {
			t.Errorf("Failed to remove non-repo directory: %v", err)
		}
	})

	// Create a parent directory
	parentDir, err := os.MkdirTemp("", "parent-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := os.RemoveAll(parentDir); err != nil {
			t.Errorf("Failed to remove parent directory: %v", err)
		}
	})

	// Move test repositories into parent directory
	require.NoError(t, os.Rename(repo1Dir, filepath.Join(parentDir, "repo1")))
	require.NoError(t, os.Rename(repo2Dir, filepath.Join(parentDir, "repo2")))
	require.NoError(t, os.Rename(nonRepoDir, filepath.Join(parentDir, "nonrepo")))

	// Test finding repositories
	var count int
	repos, err := FindRepositories([]string{parentDir}, func(c int) {
		count = c
	})
	require.NoError(t, err)
	assert.Equal(t, 2, len(repos))
	assert.Equal(t, 2, count)
}

func TestRepository_Update(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(t *testing.T) (*Repository, func())
		modifyRepo  func(t *testing.T, repo *Repository)
		wantErr     bool
		wantErrType error
	}{
		{
			name: "clean repository",
			setupRepo: func(t *testing.T) (*Repository, func()) {
				t.Log("Setting up clean repository test")
				dir, cleanup := setupTestRepo(t)
				t.Logf("Created test repository at %s", dir)
				return &Repository{Path: dir}, cleanup
			},
			wantErr: false,
		},
		{
			name: "uncommitted changes to tracked file",
			setupRepo: func(t *testing.T) (*Repository, func()) {
				t.Log("Setting up repository with uncommitted changes")
				dir, cleanup := setupTestRepo(t)
				t.Logf("Created test repository at %s", dir)
				return &Repository{Path: dir}, cleanup
			},
			modifyRepo: func(t *testing.T, repo *Repository) {
				t.Log("Creating uncommitted changes")
				// Modify the existing test.txt file to create uncommitted changes
				err := os.WriteFile(filepath.Join(repo.Path, "test.txt"), []byte("modified content"), 0644)
				require.NoError(t, err)
				t.Log("Modified tracked file: test.txt")
			},
			wantErr:     true,
			wantErrType: ErrUncommittedChanges,
		},
		{
			name: "untracked files only",
			setupRepo: func(t *testing.T) (*Repository, func()) {
				t.Log("Setting up untracked files test")
				dir, cleanup := setupTestRepo(t)
				t.Logf("Created test repository at %s", dir)
				return &Repository{Path: dir}, cleanup
			},
			modifyRepo: func(t *testing.T, repo *Repository) {
				t.Log("Creating untracked file")
				// Create a new untracked file
				err := os.WriteFile(filepath.Join(repo.Path, "untracked.txt"), []byte("untracked content"), 0644)
				require.NoError(t, err)
				t.Log("Created untracked file: untracked.txt")
			},
			wantErr: false,
		},
		{
			name: "with upstream remote",
			setupRepo: func(t *testing.T) (*Repository, func()) {
				t.Log("Setting up repository with upstream remote")
				dir, originDir, upstreamDir, cleanup := setupTestRepoWithRemotes(t)
				t.Logf("Created test repository at %s", dir)
				t.Logf("Created origin repository at %s", originDir)
				t.Logf("Created upstream repository at %s", upstreamDir)
				return &Repository{Path: dir, HasUpstream: true}, cleanup
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Running test case: %s", tt.name)
			repo, cleanup := tt.setupRepo(t)
			defer cleanup()

			if tt.modifyRepo != nil {
				t.Log("Modifying repository")
				tt.modifyRepo(t, repo)
			}

			t.Log("Updating repository")
			err := repo.Update()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrType != nil {
					assert.ErrorIs(t, err, tt.wantErrType)
				}
				t.Logf("Got expected error: %v", err)
			} else {
				assert.NoError(t, err)
				t.Log("Repository updated successfully")
			}
		})
	}
}

func TestRepository_Update_HonorsGitConfig(t *testing.T) {
	testGitEnv(t)
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	originURL := strings.TrimSpace(runGitCmd(t, dir, "remote", "get-url", "origin"))
	runGitCmd(t, dir, "remote", "set-url", "origin", "https://example.invalid/origin.git")

	// Only the insteadOf rule below can resolve that URL, so the update fails
	// unless gogitup honours the user's git configuration.
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	rule := fmt.Sprintf("[url \"%s\"]\n\tinsteadOf = https://example.invalid/origin.git\n", originURL)
	require.NoError(t, os.WriteFile(globalConfig, []byte(rule), 0644))
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)

	// Advance the origin so the update has something to fast-forward
	pushDir := filepath.Join(t.TempDir(), "pusher")
	runGitCmd(t, filepath.Dir(pushDir), "clone", originURL, pushDir)
	configureUser(t, pushDir)
	require.NoError(t, os.WriteFile(filepath.Join(pushDir, "new.txt"), []byte("new"), 0644))
	runGitCmd(t, pushDir, "add", "new.txt")
	runGitCmd(t, pushDir, "commit", "-m", "New commit")
	runGitCmd(t, pushDir, "push", "origin", "main")

	r := &Repository{Path: dir}
	require.NoError(t, r.Update())

	assert.Equal(t,
		strings.TrimSpace(runGitCmd(t, originURL, "rev-parse", "HEAD")),
		strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD")),
	)
	assert.Equal(t, "new.txt", strings.TrimSpace(runGitCmd(t, dir, "show", "--name-only", "--pretty=format:", "HEAD")))
	assert.NotEmpty(t, r.DiffStats)
}

func TestRepository_Update_DivergedBranch(t *testing.T) {
	testGitEnv(t)
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// A local commit origin does not have
	configureUser(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "local.txt"), []byte("local"), 0644))
	runGitCmd(t, dir, "add", "local.txt")
	runGitCmd(t, dir, "commit", "-m", "Local commit")

	// And an unrelated commit in origin
	originURL := strings.TrimSpace(runGitCmd(t, dir, "remote", "get-url", "origin"))
	pushDir := filepath.Join(t.TempDir(), "pusher")
	runGitCmd(t, filepath.Dir(pushDir), "clone", originURL, pushDir)
	configureUser(t, pushDir)
	require.NoError(t, os.WriteFile(filepath.Join(pushDir, "remote.txt"), []byte("remote"), 0644))
	runGitCmd(t, pushDir, "add", "remote.txt")
	runGitCmd(t, pushDir, "commit", "-m", "Remote commit")
	runGitCmd(t, pushDir, "push", "origin", "main")

	err := (&Repository{Path: dir}).Update()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "diverged")
}

func TestRepository_WithFetchHint(t *testing.T) {
	tests := []struct {
		name    string
		errText string
		want    bool
	}{
		{
			name:    "bad object from a dangling ref",
			errText: "fatal: bad object refs/remotes/origin/gtabularize\nerror: https://github.com/godlygeek/tabular.git did not send all necessary objects",
			want:    true,
		},
		{
			name:    "ref without a valid object",
			errText: "error: refs/remotes/origin/v1.0.x does not point to a valid object!",
			want:    true,
		},
		{
			name:    "incomplete object store",
			errText: "error: Could not read 60f25648814f0695eeb6c1040d97adca93c4e0bb\nfatal: unresolved deltas left after unpacking\nfatal: unpack-objects failed",
			want:    true,
		},
		{
			name:    "unrelated fetch failure",
			errText: "fatal: unable to access 'https://127.0.0.1:1/org/repo.git/': Couldn't connect to server",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Repository{Path: "/repo"}
			got := r.withFetchHint(fmt.Errorf("%s", tt.errText))

			assert.Contains(t, got.Error(), tt.errText)
			if tt.want {
				assert.Contains(t, got.Error(), "repair with")
				if strings.Contains(tt.errText, "unresolved deltas") {
					assert.Contains(t, got.Error(), "--refetch")
				} else {
					assert.Contains(t, got.Error(), "update-ref --stdin")
				}
			} else {
				assert.Equal(t, tt.errText, got.Error())
			}
		})
	}
}

// TestRepository_Update_FetchHintIsRunnable checks that the repair commands in
// the hint work as printed, including on a clone that has an origin/HEAD symref.
func TestRepository_Update_FetchHintIsRunnable(t *testing.T) {
	testGitEnv(t)
	baseDir := t.TempDir()

	originDir := filepath.Join(baseDir, "origin.git")
	require.NoError(t, os.MkdirAll(originDir, 0755))
	runGitCmd(t, originDir, "init", "--bare", "--initial-branch=main")

	seedDir := filepath.Join(baseDir, "seed")
	runGitCmd(t, baseDir, "clone", originDir, seedDir)
	configureUser(t, seedDir)
	require.NoError(t, os.WriteFile(filepath.Join(seedDir, "a.txt"), []byte("a"), 0644))
	runGitCmd(t, seedDir, "add", "a.txt")
	runGitCmd(t, seedDir, "commit", "-m", "one")
	runGitCmd(t, seedDir, "push", "origin", "main")
	runGitCmd(t, seedDir, "checkout", "-b", "side")
	require.NoError(t, os.WriteFile(filepath.Join(seedDir, "s.txt"), []byte("s"), 0644))
	runGitCmd(t, seedDir, "add", "s.txt")
	runGitCmd(t, seedDir, "commit", "-m", "side")
	runGitCmd(t, seedDir, "push", "origin", "side")

	// A shallow clone with an origin/HEAD symref, plus a remote-tracking ref
	// pointing at a commit the clone never fetched
	workDir := filepath.Join(baseDir, "work")
	runGitCmd(t, baseDir, "clone", "--depth", "1", "--single-branch", "--branch", "main", "file://"+originDir, workDir)
	configureUser(t, workDir)
	runGitCmd(t, workDir, "remote", "set-head", "origin", "main")

	sideHash := strings.TrimSpace(runGitCmd(t, originDir, "rev-parse", "side"))
	refPath := filepath.Join(workDir, ".git", "refs", "remotes", "origin", "side")
	require.NoError(t, os.MkdirAll(filepath.Dir(refPath), 0755))
	require.NoError(t, os.WriteFile(refPath, []byte(sideHash+"\n"), 0644))

	r := &Repository{Path: workDir}
	err := r.Update()
	require.Error(t, err)
	require.Contains(t, err.Error(), "repair with")

	// Run every command the hint prints, as printed
	commands := 0
	for _, line := range strings.Split(err.Error(), "\n") {
		if !strings.HasPrefix(line, "  git -C ") {
			continue
		}
		cmd := exec.Command("sh", "-c", strings.TrimSpace(line))
		out, cmdErr := cmd.CombinedOutput()
		require.NoError(t, cmdErr, "hint command failed: %s: %s", line, string(out))
		commands++
	}
	require.Greater(t, commands, 0, "expected the hint to contain git commands")

	// After running them the repository must update normally
	require.NoError(t, (&Repository{Path: workDir}).Update())
}

func TestCredentialArgs(t *testing.T) {
	t.Run("no token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		assert.Empty(t, credentialArgs())
	})

	t.Run("token set", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		args := credentialArgs()
		require.Len(t, args, 2)
		assert.Equal(t, "-c", args[0])
		assert.Equal(t, `credential.https://github.com.helper=!f() { echo username=x-access-token; echo "password=$GITHUB_TOKEN"; }; f`, args[1])
		// The token is expanded by git at runtime, never baked into argv.
		assert.NotContains(t, strings.Join(args, " "), "test-token")
	})
}

func TestRepository_Update_Errors(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (*Repository, func())
		expectError string
	}{
		{
			name: "invalid worktree",
			setup: func(t *testing.T) (*Repository, func()) {
				testGitEnv(t)
				dir := t.TempDir()
				runGitCmd(t, dir, "init", "--initial-branch=main")
				// Break the repository by removing HEAD
				require.NoError(t, os.Remove(filepath.Join(dir, ".git", "HEAD")))
				return &Repository{Path: dir}, func() {}
			},
			expectError: "not a git repository",
		},
		{
			name: "uncommitted changes",
			setup: func(t *testing.T) (*Repository, func()) {
				dir, cleanup := setupTestRepo(t)
				err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("modified content"), 0644)
				require.NoError(t, err)
				return &Repository{Path: dir}, cleanup
			},
			expectError: "worktree contains uncommitted changes to tracked files",
		},
		{
			name: "unreachable remote",
			setup: func(t *testing.T) (*Repository, func()) {
				dir, cleanup := setupTestRepo(t)
				runGitCmd(t, dir, "remote", "set-url", "origin", "https://127.0.0.1:1/user/repo.git")
				return &Repository{Path: dir}, cleanup
			},
			expectError: "failed to fetch from origin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, cleanup := tt.setup(t)
			defer cleanup()

			err := repo.Update()
			if tt.expectError != "" {
				assert.ErrorContains(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetCacheFile(t *testing.T) {
	// Save original cache dir and restore after test
	origCacheDir := os.Getenv("XDG_CACHE_HOME")
	origHomeDir := os.Getenv("HOME")
	defer func() {
		err := os.Setenv("XDG_CACHE_HOME", origCacheDir)
		require.NoError(t, err)
		err = os.Setenv("HOME", origHomeDir)
		require.NoError(t, err)
	}()

	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "gogitup-test-cache-*")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Set XDG_CACHE_HOME to override default cache location
	err = os.Setenv("XDG_CACHE_HOME", tmpDir)
	require.NoError(t, err)
	err = os.Setenv("HOME", tmpDir)
	require.NoError(t, err)

	// Get cache file path
	cacheFile, err := GetCacheFile()
	require.NoError(t, err)

	// On macOS, os.UserCacheDir() returns Library/Caches
	// On other platforms, it uses XDG_CACHE_HOME
	var expected string
	if runtime.GOOS == "darwin" {
		expected = filepath.Join(tmpDir, "Library", "Caches", "gogitup", "repositories.json")
	} else {
		expected = filepath.Join(tmpDir, "gogitup", "repositories.json")
	}

	assert.Equal(t, expected, cacheFile)

	// Verify directory was created
	_, err = os.Stat(filepath.Dir(cacheFile))
	assert.NoError(t, err)
}

func TestSaveRepositories(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "gogitup-test-save-*")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	reposFile := filepath.Join(tmpDir, "repos.json")
	viper.Set("repos-file", reposFile)
	defer viper.Reset()

	// Test saving empty list
	err = SaveRepositories([]Repository{})
	require.NoError(t, err)
	assertFileExists(t, reposFile)

	// Test saving with repositories
	repos := []Repository{
		{Path: "/path/to/repo1", HasUpstream: true},
		{Path: "/path/to/repo2", HasUpstream: false},
	}
	err = SaveRepositories(repos)
	require.NoError(t, err)

	// Verify file contents
	data, err := os.ReadFile(reposFile)
	require.NoError(t, err)

	var savedRepos []Repository
	err = json.Unmarshal(data, &savedRepos)
	require.NoError(t, err)

	assert.Equal(t, 2, len(savedRepos))
	assert.Equal(t, "/path/to/repo1", savedRepos[0].Path)
	assert.Equal(t, true, savedRepos[0].HasUpstream)
	assert.Equal(t, "/path/to/repo2", savedRepos[1].Path)
	assert.Equal(t, false, savedRepos[1].HasUpstream)
	assert.False(t, savedRepos[0].LastScanned.IsZero())
	assert.False(t, savedRepos[1].LastScanned.IsZero())

	// Test saving to invalid path
	viper.Set("repos-file", "/invalid/path/repos.json")
	err = SaveRepositories(repos)
	assert.Error(t, err)
}

func TestLoadRepositories(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "gogitup-test-load-*")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	reposFile := filepath.Join(tmpDir, "repos.json")
	viper.Set("repos-file", reposFile)
	defer viper.Reset()

	// Test loading non-existent file
	repos, err := LoadRepositories()
	require.NoError(t, err)
	assert.Empty(t, repos)

	// Create test repository
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Save test repository
	testRepos := []Repository{
		{Path: repoDir, HasUpstream: false, LastScanned: time.Now()},
	}
	data, err := json.MarshalIndent(testRepos, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(reposFile, data, 0644)
	require.NoError(t, err)

	// Test loading valid repository
	repos, err = LoadRepositories()
	require.NoError(t, err)
	assert.Equal(t, 1, len(repos))
	assert.Equal(t, repoDir, repos[0].Path)

	// Test loading invalid repository path
	invalidPath := filepath.Join(tmpDir, "invalid")
	testRepos = []Repository{
		{Path: invalidPath, HasUpstream: false, LastScanned: time.Now()},
	}
	data, err = json.MarshalIndent(testRepos, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(reposFile, data, 0644)
	require.NoError(t, err)

	// LoadRepositories should skip invalid repositories
	repos, err = LoadRepositories()
	require.NoError(t, err)
	assert.Empty(t, repos)

	// Test loading invalid JSON
	err = os.WriteFile(reposFile, []byte("invalid json"), 0644)
	require.NoError(t, err)

	repos, err = LoadRepositories()
	assert.Error(t, err)
	assert.Nil(t, repos)
}

func TestFindRepositories_EdgeCases(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "gogitup-test-find-*")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	tests := []struct {
		name        string
		setup       func(t *testing.T) []string
		expectErr   bool
		expectRepos int
	}{
		{
			name: "non-existent directory",
			setup: func(t *testing.T) []string {
				return []string{"/path/that/does/not/exist"}
			},
			expectErr:   false,
			expectRepos: 0,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) []string {
				emptyDir := filepath.Join(tmpDir, "empty")
				require.NoError(t, os.MkdirAll(emptyDir, 0755))
				return []string{emptyDir}
			},
			expectErr:   false,
			expectRepos: 0,
		},
		{
			name: "directory with .git but not a repo",
			setup: func(t *testing.T) []string {
				invalidRepo := filepath.Join(tmpDir, "invalid")
				require.NoError(t, os.MkdirAll(filepath.Join(invalidRepo, ".git"), 0755))
				return []string{invalidRepo}
			},
			expectErr:   false,
			expectRepos: 0,
		},
		{
			name: "directory with unreadable .git",
			setup: func(t *testing.T) []string {
				unreadableRepo := filepath.Join(tmpDir, "unreadable")
				require.NoError(t, os.MkdirAll(unreadableRepo, 0755))
				gitDir := filepath.Join(unreadableRepo, ".git")
				require.NoError(t, os.MkdirAll(gitDir, 0755))
				require.NoError(t, os.Chmod(gitDir, 0000))
				t.Cleanup(func() {
					// Restore permissions to allow cleanup
					err := os.Chmod(gitDir, 0755)
					require.NoError(t, err)
				})
				return []string{unreadableRepo}
			},
			expectErr:   false,
			expectRepos: 0,
		},
		{
			name: "multiple valid repositories",
			setup: func(t *testing.T) []string {
				repo1, cleanup1 := setupTestRepo(t)
				t.Cleanup(cleanup1)
				repo2, cleanup2 := setupTestRepo(t)
				t.Cleanup(cleanup2)
				return []string{repo1, repo2}
			},
			expectErr:   false,
			expectRepos: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := tt.setup(t)
			var count int
			repos, err := FindRepositories(dirs, func(c int) { count = c })

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectRepos, len(repos))
				assert.Equal(t, tt.expectRepos, count)
			}
		})
	}
}

func assertFileExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	assert.NoError(t, err)
}

func TestRepository_isLFSRepository(t *testing.T) {
	tests := []struct {
		name          string
		gitattributes string
		expectUsesLFS bool
	}{
		{
			name:          "no gitattributes file",
			expectUsesLFS: false,
		},
		{
			name:          "empty gitattributes file",
			gitattributes: "",
			expectUsesLFS: false,
		},
		{
			name:          "gitattributes without LFS",
			gitattributes: "*.txt text=auto\n*.sh text eol=lf\n",
			expectUsesLFS: false,
		},
		{
			name:          "gitattributes with LFS",
			gitattributes: "*.bin filter=lfs diff=lfs merge=lfs -text\n*.txt text=auto\n",
			expectUsesLFS: true,
		},
		{
			name:          "gitattributes with only LFS filter",
			gitattributes: "*.data filter=lfs\n",
			expectUsesLFS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			dir, err := os.MkdirTemp("", "gogitup-test-lfs-*")
			require.NoError(t, err)
			defer func() {
				if err := os.RemoveAll(dir); err != nil {
					t.Errorf("Failed to remove temp directory: %v", err)
				}
			}()

			// Initialize git repository
			testGitEnv(t)
			runGitCmd(t, dir, "init", "--initial-branch=main")

			// Create .gitattributes file if content is provided
			if tt.gitattributes != "" {
				err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(tt.gitattributes), 0644)
				require.NoError(t, err)
			}

			// Create repository instance
			r := &Repository{Path: dir}

			// Test LFS detection
			assert.Equal(t, tt.expectUsesLFS, r.isLFSRepository())
		})
	}
}

// Add this helper function before TestRepository_UpdateLFS
func getDefaultBranch(t *testing.T, dir string) string {
	t.Helper()
	// Try to get the default branch from git config
	cmd := exec.Command("git", "config", "--get", "init.defaultBranch")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		return strings.TrimSpace(string(out))
	}
	// Fallback to checking if we're on main or master
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", branch)
		cmd.Dir = dir
		if err := cmd.Run(); err == nil {
			return branch
		}
	}
	// Default to main if nothing else is found
	return "main"
}

func TestRepository_UpdateLFS(t *testing.T) {
	if os.Getenv("SKIP_GIT_LFS_TESTS") != "" {
		t.Skip("Skipping Git LFS tests (SKIP_GIT_LFS_TESTS is set)")
	}

	// Check if git-lfs is installed
	if _, err := exec.LookPath("git-lfs"); err != nil {
		t.Skip("git-lfs is not installed")
	}

	tests := []struct {
		name           string
		setupRepo      func(t *testing.T, dir string) error
		modifyRepo     func(t *testing.T, dir string) error
		hasUpstream    bool
		expectError    bool
		expectDiffStat bool
	}{
		{
			name: "clean LFS repository",
			setupRepo: func(t *testing.T, dir string) error {
				// Create a bare repository for origin
				originDir := dir + "-origin"
				if err := os.MkdirAll(originDir, 0755); err != nil {
					return err
				}
				cmd := exec.Command("git", "init", "--bare")
				cmd.Dir = originDir
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to init bare repo: %s: %w", string(out), err)
				}

				// Initialize git repository with detected default branch
				defaultBranch := getDefaultBranch(t, originDir)
				cmd = exec.Command("git", "init", "-b", defaultBranch)
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to init repo: %s: %w", string(out), err)
				}

				// Initialize LFS
				cmd = exec.Command("git", "lfs", "install")
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to install LFS: %s: %w", string(out), err)
				}

				// Create .gitattributes
				if err := os.WriteFile(filepath.Join(dir, ".gitattributes"),
					[]byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0644); err != nil {
					return err
				}

				// Create and track a binary file
				if err := os.WriteFile(filepath.Join(dir, "test.bin"), []byte{0, 1, 2, 3}, 0644); err != nil {
					return err
				}

				// Add and commit
				cmds := [][]string{
					{"config", "user.name", "Test User"},
					{"config", "user.email", "test@example.com"},
					{"add", ".gitattributes"},
					{"add", "test.bin"},
					{"commit", "-m", "Initial commit with LFS"},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if err := cmd.Run(); err != nil {
						return err
					}
				}

				// Add origin remote and push
				cmds = [][]string{
					{"remote", "add", "origin", originDir},
					{"push", "-u", "origin", defaultBranch},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if out, err := cmd.CombinedOutput(); err != nil {
						return fmt.Errorf("failed to run git %s: %s: %w", strings.Join(args, " "), string(out), err)
					}
				}

				return nil
			},
			expectError:    false,
			expectDiffStat: false,
		},
		{
			name: "LFS repository with changes",
			setupRepo: func(t *testing.T, dir string) error {
				// Create a bare repository for origin
				originDir := dir + "-origin"
				if err := os.MkdirAll(originDir, 0755); err != nil {
					return fmt.Errorf("failed to create origin dir: %w", err)
				}
				cmd := exec.Command("git", "init", "--bare")
				cmd.Dir = originDir
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to init bare repo: %s: %w", string(out), err)
				}

				// Initialize git repository with detected default branch
				defaultBranch := getDefaultBranch(t, originDir)
				cmd = exec.Command("git", "init", "-b", defaultBranch)
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to init repo: %s: %w", string(out), err)
				}

				// Initialize LFS
				cmd = exec.Command("git", "lfs", "install")
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to install LFS: %s: %w", string(out), err)
				}

				// Create .gitattributes
				if err := os.WriteFile(filepath.Join(dir, ".gitattributes"),
					[]byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0644); err != nil {
					return fmt.Errorf("failed to write .gitattributes: %w", err)
				}

				// Create and track a binary file
				if err := os.WriteFile(filepath.Join(dir, "test.bin"), []byte{0, 1, 2, 3}, 0644); err != nil {
					return fmt.Errorf("failed to write test.bin: %w", err)
				}

				// Add and commit
				cmds := [][]string{
					{"config", "user.name", "Test User"},
					{"config", "user.email", "test@example.com"},
					{"add", ".gitattributes"},
					{"add", "test.bin"},
					{"commit", "-m", "Initial commit with LFS"},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if out, err := cmd.CombinedOutput(); err != nil {
						return fmt.Errorf("failed to run git %s: %s: %w", strings.Join(args, " "), string(out), err)
					}
				}

				// Add origin remote and push
				cmds = [][]string{
					{"remote", "add", "origin", originDir},
					{"push", "-u", "origin", defaultBranch},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if out, err := cmd.CombinedOutput(); err != nil {
						return fmt.Errorf("failed to run git %s: %s: %w", strings.Join(args, " "), string(out), err)
					}
				}

				// Create a new binary file in a clone
				cloneDir := dir + "-clone"
				cmd = exec.Command("git", "clone", originDir, cloneDir)
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to clone repo: %s: %w", string(out), err)
				}
				defer func() {
					if err := os.RemoveAll(cloneDir); err != nil {
						t.Errorf("Failed to remove clone directory: %v", err)
					}
				}()

				// Get the default branch name from the origin
				defaultBranch = getDefaultBranch(t, originDir)

				// Ensure we're on the correct branch
				cmd = exec.Command("git", "checkout", defaultBranch)
				cmd.Dir = cloneDir
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to checkout branch: %s: %w", string(out), err)
				}

				// Initialize LFS in clone and set up git config
				cmds = [][]string{
					{"lfs", "install"},
					{"config", "user.name", "Test User"},
					{"config", "user.email", "test@example.com"},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = cloneDir
					if out, err := cmd.CombinedOutput(); err != nil {
						return fmt.Errorf("failed to run git %s: %s: %w", strings.Join(args, " "), string(out), err)
					}
				}

				// Create and commit a new file
				if err := os.WriteFile(filepath.Join(cloneDir, "new.bin"), []byte{4, 5, 6, 7}, 0644); err != nil {
					return fmt.Errorf("failed to write new.bin: %w", err)
				}

				// Add and commit the new file
				cmds = [][]string{
					{"add", "new.bin"},
					{"commit", "-m", "Add new binary file"},
					{"push", "origin", defaultBranch},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = cloneDir
					if out, err := cmd.CombinedOutput(); err != nil {
						return fmt.Errorf("failed to run git %s: %s: %w", strings.Join(args, " "), string(out), err)
					}
				}

				return nil
			},
			expectError:    false,
			expectDiffStat: true,
		},
		{
			name: "LFS repository with uncommitted changes to tracked file",
			setupRepo: func(t *testing.T, dir string) error {
				// Create a bare repository for origin
				originDir := dir + "-origin"
				if err := os.MkdirAll(originDir, 0755); err != nil {
					return err
				}
				cmd := exec.Command("git", "init", "--bare")
				cmd.Dir = originDir
				if err := cmd.Run(); err != nil {
					return err
				}

				// Initialize git repository with detected default branch
				defaultBranch := getDefaultBranch(t, dir)
				cmd = exec.Command("git", "init", "-b", defaultBranch)
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					return err
				}

				// Initialize LFS
				cmd = exec.Command("git", "lfs", "install")
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					return err
				}

				// Create .gitattributes
				if err := os.WriteFile(filepath.Join(dir, ".gitattributes"),
					[]byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0644); err != nil {
					return err
				}

				// Create and track a binary file
				if err := os.WriteFile(filepath.Join(dir, "test.bin"), []byte{0, 1, 2, 3}, 0644); err != nil {
					return err
				}

				// Add and commit
				cmds := [][]string{
					{"config", "user.name", "Test User"},
					{"config", "user.email", "test@example.com"},
					{"add", ".gitattributes"},
					{"add", "test.bin"},
					{"commit", "-m", "Initial commit with LFS"},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if err := cmd.Run(); err != nil {
						return err
					}
				}

				// Add origin remote and push
				cmds = [][]string{
					{"remote", "add", "origin", originDir},
					{"push", "-u", "origin", defaultBranch},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if err := cmd.Run(); err != nil {
						return err
					}
				}

				return nil
			},
			modifyRepo: func(t *testing.T, dir string) error {
				// Modify the binary file
				if err := os.WriteFile(filepath.Join(dir, "test.bin"), []byte{8, 9, 10, 11}, 0644); err != nil {
					return err
				}
				return nil
			},
			expectError:    true,
			expectDiffStat: false,
		},
		{
			name: "LFS repository with untracked files only",
			setupRepo: func(t *testing.T, dir string) error {
				// Create a bare repository for origin
				originDir := dir + "-origin"
				if err := os.MkdirAll(originDir, 0755); err != nil {
					return err
				}
				cmd := exec.Command("git", "init", "--bare")
				cmd.Dir = originDir
				if err := cmd.Run(); err != nil {
					return err
				}

				// Initialize git repository with detected default branch
				defaultBranch := getDefaultBranch(t, dir)
				cmd = exec.Command("git", "init", "-b", defaultBranch)
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					return err
				}

				// Initialize LFS
				cmd = exec.Command("git", "lfs", "install")
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					return err
				}

				// Create .gitattributes
				if err := os.WriteFile(filepath.Join(dir, ".gitattributes"),
					[]byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0644); err != nil {
					return err
				}

				// Create and track a binary file
				if err := os.WriteFile(filepath.Join(dir, "test.bin"), []byte{0, 1, 2, 3}, 0644); err != nil {
					return err
				}

				// Add and commit
				cmds := [][]string{
					{"config", "user.name", "Test User"},
					{"config", "user.email", "test@example.com"},
					{"add", ".gitattributes"},
					{"add", "test.bin"},
					{"commit", "-m", "Initial commit with LFS"},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if err := cmd.Run(); err != nil {
						return err
					}
				}

				// Add origin remote and push
				cmds = [][]string{
					{"remote", "add", "origin", originDir},
					{"push", "-u", "origin", defaultBranch},
				}
				for _, args := range cmds {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					if err := cmd.Run(); err != nil {
						return err
					}
				}

				return nil
			},
			modifyRepo: func(t *testing.T, dir string) error {
				// Create an untracked file
				if err := os.WriteFile(filepath.Join(dir, "untracked.bin"), []byte{12, 13, 14, 15}, 0644); err != nil {
					return err
				}
				return nil
			},
			expectError:    false,
			expectDiffStat: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test repository
			dir, err := os.MkdirTemp("", "gogitup-test")
			require.NoError(t, err)
			defer func() {
				if err := os.RemoveAll(dir); err != nil {
					t.Errorf("Failed to remove test directory: %v", err)
				}
			}()

			// Set up repository
			err = tt.setupRepo(t, dir)
			require.NoError(t, err)

			// Create repository instance
			r := &Repository{Path: dir}

			// Modify repository if needed
			if tt.modifyRepo != nil {
				err = tt.modifyRepo(t, dir)
				require.NoError(t, err)
			}

			// Update repository
			err = r.Update()
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, ErrUncommittedChanges, err)
			} else {
				assert.NoError(t, err)
				if tt.expectDiffStat {
					assert.NotEmpty(t, r.DiffStats)
				} else {
					assert.Empty(t, r.DiffStats)
				}
			}
		})
	}
}
