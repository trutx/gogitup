package git

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
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	// Create temporary directories for origin and local repos
	originDir, err := os.MkdirTemp("", "gogitup-test-origin-*")
	require.NoError(t, err)
	localDir, err := os.MkdirTemp("", "gogitup-test-*")
	require.NoError(t, err)
	t.Logf("Created test directories: origin=%s, local=%s", originDir, localDir)

	// Initialize a bare repository for origin
	_, err = git.PlainInit(originDir, true)
	require.NoError(t, err)
	t.Log("Initialized bare origin repository")

	// Initialize the local repository
	repo, err := git.PlainInit(localDir, false)
	require.NoError(t, err)
	t.Log("Initialized local repository")

	// Create a test file and commit it
	testFile := filepath.Join(localDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)
	t.Log("Created test file: test.txt")

	// Get the worktree
	w, err := repo.Worktree()
	require.NoError(t, err)

	// Add and commit the file
	_, err = w.Add("test.txt")
	require.NoError(t, err)
	t.Log("Added test file to index")

	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)
	t.Log("Created initial commit")

	// Add origin remote
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{originDir},
	})
	require.NoError(t, err)
	t.Log("Added origin remote")

	// Push to origin
	err = repo.Push(&git.PushOptions{})
	require.NoError(t, err)
	t.Log("Pushed to origin")

	// Create a cleanup function
	cleanup := func() {
		if err := os.RemoveAll(localDir); err != nil {
			t.Errorf("Failed to remove local directory: %v", err)
		}
		err = os.RemoveAll(originDir)
		if err != nil {
			t.Errorf("Failed to remove origin directory: %v", err)
		}
		t.Log("Cleaned up test directories")
	}

	return localDir, cleanup
}

func setupTestRepoWithRemotes(t *testing.T) (string, string, string, func()) {
	t.Helper()

	// Create temporary directories for origin, upstream, and local repos
	originDir, err := os.MkdirTemp("", "gogitup-test-origin-*")
	require.NoError(t, err)
	upstreamDir, err := os.MkdirTemp("", "gogitup-test-upstream-*")
	require.NoError(t, err)
	localDir, err := os.MkdirTemp("", "gogitup-test-local-*")
	require.NoError(t, err)
	t.Logf("Created test directories: origin=%s, upstream=%s, local=%s", originDir, upstreamDir, localDir)

	// Initialize upstream repository
	upstreamRepo, err := git.PlainInit(upstreamDir, false)
	require.NoError(t, err)
	t.Log("Initialized upstream repository")

	// Create and commit a test file in upstream
	testFile := filepath.Join(upstreamDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)
	t.Log("Created test file in upstream: test.txt")

	w, err := upstreamRepo.Worktree()
	require.NoError(t, err)

	_, err = w.Add("test.txt")
	require.NoError(t, err)
	t.Log("Added test file to upstream index")

	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)
	t.Log("Created initial commit in upstream")

	// Initialize origin repository as bare
	_, err = git.PlainInit(originDir, true)
	require.NoError(t, err)
	t.Log("Initialized bare origin repository")

	// Clone upstream to local
	repo, err := git.PlainClone(localDir, false, &git.CloneOptions{
		URL: upstreamDir,
	})
	require.NoError(t, err)
	t.Log("Cloned upstream to local")

	// Set up remotes for fork workflow
	_, err = repo.Remote("origin")
	require.NoError(t, err)
	t.Log("Got origin remote")

	// Rename origin to upstream
	err = repo.DeleteRemote("origin")
	require.NoError(t, err)
	t.Log("Deleted origin remote")

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "upstream",
		URLs: []string{upstreamDir},
	})
	require.NoError(t, err)
	t.Log("Added upstream remote")

	// Add origin remote
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{originDir},
	})
	require.NoError(t, err)
	t.Log("Added origin remote")

	// Push to origin
	err = repo.Push(&git.PushOptions{})
	require.NoError(t, err)
	t.Log("Pushed to origin")

	// Create a new commit in upstream
	err = os.WriteFile(filepath.Join(upstreamDir, "upstream.txt"), []byte("upstream content"), 0644)
	require.NoError(t, err)
	t.Log("Created new file in upstream: upstream.txt")

	w, err = upstreamRepo.Worktree()
	require.NoError(t, err)

	_, err = w.Add("upstream.txt")
	require.NoError(t, err)
	t.Log("Added new file to upstream index")

	_, err = w.Commit("Upstream commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)
	t.Log("Created new commit in upstream")

	// Create a cleanup function
	cleanup := func() {
		if err := os.RemoveAll(originDir); err != nil {
			t.Errorf("Failed to remove origin directory: %v", err)
		}
		err = os.RemoveAll(upstreamDir)
		if err != nil {
			t.Errorf("Failed to remove upstream directory: %v", err)
		}
		if err := os.RemoveAll(localDir); err != nil {
			t.Errorf("Failed to remove local directory: %v", err)
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
				repo, err := git.PlainOpen(dir)
				require.NoError(t, err)
				t.Logf("Created test repository at %s", dir)
				return &Repository{Path: dir, repo: repo}, cleanup
			},
			wantErr: false,
		},
		{
			name: "unstaged changes",
			setupRepo: func(t *testing.T) (*Repository, func()) {
				t.Log("Setting up unstaged changes test")
				dir, cleanup := setupTestRepo(t)
				repo, err := git.PlainOpen(dir)
				require.NoError(t, err)
				t.Logf("Created test repository at %s", dir)
				return &Repository{Path: dir, repo: repo}, cleanup
			},
			modifyRepo: func(t *testing.T, repo *Repository) {
				t.Log("Creating unstaged changes")
				// Create a new file to cause unstaged changes
				err := os.WriteFile(filepath.Join(repo.Path, "unstaged.txt"), []byte("unstaged content"), 0644)
				require.NoError(t, err)
				t.Log("Created unstaged file: unstaged.txt")
			},
			wantErr:     true,
			wantErrType: ErrUnstagedChanges,
		},
		{
			name: "with upstream remote",
			setupRepo: func(t *testing.T) (*Repository, func()) {
				t.Log("Setting up repository with upstream remote")
				dir, originDir, upstreamDir, cleanup := setupTestRepoWithRemotes(t)
				repo, err := git.PlainOpen(dir)
				require.NoError(t, err)
				t.Logf("Created test repository at %s", dir)
				t.Logf("Created origin repository at %s", originDir)
				t.Logf("Created upstream repository at %s", upstreamDir)
				return &Repository{Path: dir, repo: repo, HasUpstream: true}, cleanup
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

func TestRepository_GetAuth(t *testing.T) {
	// Save original GITHUB_TOKEN and restore after test
	origToken := os.Getenv("GITHUB_TOKEN")
	defer func() {
		err := os.Setenv("GITHUB_TOKEN", origToken)
		require.NoError(t, err)
	}()

	tests := []struct {
		name     string
		setup    func() *Repository
		wantAuth bool
	}{
		{
			name: "github repository with token",
			setup: func() *Repository {
				err := os.Setenv("GITHUB_TOKEN", "test-token")
				require.NoError(t, err)
				return &Repository{Path: "/path/to/github.com/user/repo"}
			},
			wantAuth: true,
		},
		{
			name: "github repository without token",
			setup: func() *Repository {
				err := os.Unsetenv("GITHUB_TOKEN")
				require.NoError(t, err)
				return &Repository{Path: "/path/to/github.com/user/repo"}
			},
			wantAuth: false,
		},
		{
			name: "non-github repository",
			setup: func() *Repository {
				return &Repository{Path: "/path/to/gitlab.com/user/repo"}
			},
			wantAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setup()
			auth := repo.getAuth()
			if tt.wantAuth {
				assert.NotNil(t, auth)
				if basicAuth, ok := auth.(*githttp.BasicAuth); ok {
					assert.Equal(t, "git", basicAuth.Username)
					assert.Equal(t, "test-token", basicAuth.Password)
				} else {
					t.Error("Expected BasicAuth type")
				}
			} else {
				assert.Nil(t, auth)
			}
		})
	}
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
				dir, err := os.MkdirTemp("", "gogitup-test-invalid-*")
				require.NoError(t, err)
				repo, err := git.PlainInit(dir, false)
				require.NoError(t, err)
				// Create an invalid repository state
				err = os.RemoveAll(filepath.Join(dir, ".git", "HEAD"))
				require.NoError(t, err)
				return &Repository{Path: dir, repo: repo}, func() {
					if err := os.RemoveAll(dir); err != nil {
						t.Errorf("Failed to remove directory: %v", err)
					}
				}
			},
			expectError: "reference not found",
		},
		{
			name: "unstaged changes",
			setup: func(t *testing.T) (*Repository, func()) {
				dir, cleanup := setupTestRepo(t)
				repo, err := git.PlainOpen(dir)
				require.NoError(t, err)
				// Create unstaged file
				err = os.WriteFile(filepath.Join(dir, "unstaged.txt"), []byte("unstaged"), 0644)
				require.NoError(t, err)
				return &Repository{Path: dir, repo: repo}, cleanup
			},
			expectError: "worktree contains unstaged changes",
		},
		{
			name: "authentication required",
			setup: func(t *testing.T) (*Repository, func()) {
				dir, cleanup := setupTestRepo(t)
				repo, err := git.PlainOpen(dir)
				require.NoError(t, err)
				// Remove any existing remotes
				remotes, err := repo.Remotes()
				require.NoError(t, err)
				for _, r := range remotes {
					err = repo.DeleteRemote(r.Config().Name)
					require.NoError(t, err)
				}
				// Add GitHub remote
				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{"https://github.com/user/repo.git"},
				})
				require.NoError(t, err)
				err = os.Unsetenv("GITHUB_TOKEN")
				require.NoError(t, err)
				return &Repository{Path: dir, repo: repo}, cleanup
			},
			expectError: "authentication required",
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
	assert.NotNil(t, repos[0].repo)

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
