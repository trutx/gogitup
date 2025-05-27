package git

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// Common errors
var (
	ErrUnstagedChanges = fmt.Errorf("worktree contains unstaged changes")
)

// Repository represents a Git repository
type Repository struct {
	Path        string          `json:"path"`
	HasUpstream bool            `json:"has_upstream"`
	LastScanned time.Time       `json:"last_scanned"`
	DiffStats   string          `json:"-"`
	repo        *git.Repository `json:"-"`
}

// GetCacheFile returns the default path to the cache file
func GetCacheFile() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get cache directory: %w", err)
	}

	// Create gogitup cache directory if it doesn't exist
	gogitupCache := filepath.Join(cacheDir, "gogitup")
	if err := os.MkdirAll(gogitupCache, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return filepath.Join(gogitupCache, "repositories.json"), nil
}

// SaveRepositories saves the repository list to the specified file
func SaveRepositories(repositories []Repository) error {
	reposFile := viper.GetString("repos-file")
	if reposFile == "" {
		var err error
		reposFile, err = GetCacheFile()
		if err != nil {
			return err
		}
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(reposFile), 0755); err != nil {
		return fmt.Errorf("failed to create directory for repos file: %w", err)
	}

	// Add scan timestamp
	for i := range repositories {
		repositories[i].LastScanned = time.Now()
	}

	data, err := json.MarshalIndent(repositories, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal repositories: %w", err)
	}

	if err := os.WriteFile(reposFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write repos file: %w", err)
	}

	return nil
}

// LoadRepositories loads the repository list from the specified file
func LoadRepositories() ([]Repository, error) {
	reposFile := viper.GetString("repos-file")
	if reposFile == "" {
		var err error
		reposFile, err = GetCacheFile()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(reposFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read repos file: %w", err)
	}

	var repositories []Repository
	if err := json.Unmarshal(data, &repositories); err != nil {
		return nil, fmt.Errorf("failed to unmarshal repositories: %w", err)
	}

	// Filter out repositories that can't be opened
	validRepos := make([]Repository, 0, len(repositories))
	for i := range repositories {
		repo, err := git.PlainOpen(repositories[i].Path)
		if err != nil {
			// Skip repositories that can't be opened
			continue
		}
		repositories[i].repo = repo
		validRepos = append(validRepos, repositories[i])
	}

	return validRepos, nil
}

// FindRepositories searches for Git repositories in the given directories
func FindRepositories(directories []string, onFound func(count int)) ([]Repository, error) {
	var repositories []Repository
	count := 0

	for _, dir := range directories {
		// Expand home directory if path starts with ~
		if strings.HasPrefix(dir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get user home directory: %w", err)
			}
			dir = filepath.Join(home, dir[2:])
		}

		// Skip if directory doesn't exist
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsPermission(err) {
					// Skip directories we can't access
					if info != nil && info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				return err
			}

			// Skip if not a directory
			if !info.IsDir() {
				return nil
			}

			// Check for .git directory
			gitDir := filepath.Join(path, ".git")
			if stat, err := os.Stat(gitDir); err == nil && stat.IsDir() {
				repo, err := git.PlainOpen(path)
				if err != nil {
					return nil // Skip invalid repositories
				}

				// Check for upstream remote
				remotes, err := repo.Remotes()
				if err != nil {
					return nil // Skip if can't read remotes
				}

				hasUpstream := false
				for _, remote := range remotes {
					if remote.Config().Name == "upstream" {
						hasUpstream = true
						break
					}
				}

				repositories = append(repositories, Repository{
					Path:        path,
					HasUpstream: hasUpstream,
					repo:        repo,
				})
				count++
				if onFound != nil {
					onFound(count)
				}

				return filepath.SkipDir
			}

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to walk directory %s: %w", dir, err)
		}
	}

	return repositories, nil
}

func (r *Repository) getAuth() transport.AuthMethod {
	// Check if this is a GitHub repository
	if strings.Contains(r.Path, "github.com") {
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			return &http.BasicAuth{
				Username: "git", // This can be anything except empty
				Password: token,
			}
		}
	}
	return nil
}

// getTerminalWidth returns the terminal width, defaulting to 80 if not in a terminal
func getTerminalWidth() int {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return 80
	}
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80
	}
	return width
}

// formatDiffStat formats a single diff stat line with proper width constraints
func formatDiffStat(path string, added, removed int, graphWidth int, green, red func(string) string) string {
	// Calculate the graph part
	total := added + removed
	graph := ""
	if graphWidth > 0 {
		// Scale the number of symbols to fit the graph width
		symbolCount := graphWidth
		if total > graphWidth {
			symbolCount = graphWidth
		} else if total < graphWidth {
			symbolCount = total
		}

		// Calculate proportions of + and - symbols
		plusCount := 0
		minusCount := 0
		if total > 0 {
			plusCount = (added * symbolCount) / total
			minusCount = symbolCount - plusCount
		}

		graph = green(strings.Repeat("+", plusCount)) + red(strings.Repeat("-", minusCount))
	}

	return fmt.Sprintf(" %s | %2d %s",
		path,
		total,
		graph,
	)
}

// Update updates the repository by fetching and pulling changes
func (r *Repository) Update() error {
	// Get worktree and check for unstaged changes first
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Check for unstaged changes before any operations
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}
	if !status.IsClean() {
		return ErrUnstagedChanges
	}

	// Get current HEAD for diff comparison
	head, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	oldHead := head.Hash()

	// Perform update
	var updateErr error
	if r.HasUpstream {
		updateErr = r.updateWithUpstream()
	} else {
		updateErr = r.updateOrigin()
	}

	// If update was successful, get diff stats
	if updateErr == nil {
		newHead, err := r.repo.Head()
		if err != nil {
			return fmt.Errorf("failed to get new HEAD: %w", err)
		}

		// Only get diff stats if HEAD changed
		if oldHead != newHead.Hash() {
			// Get the commits
			oldCommit, err := r.repo.CommitObject(oldHead)
			if err != nil {
				return fmt.Errorf("failed to get old commit: %w", err)
			}
			newCommit, err := r.repo.CommitObject(newHead.Hash())
			if err != nil {
				return fmt.Errorf("failed to get new commit: %w", err)
			}

			// Get the patch
			patch, err := oldCommit.Patch(newCommit)
			if err != nil {
				return fmt.Errorf("failed to get patch: %w", err)
			}

			// Calculate stats
			stats := make(map[string]struct {
				added   int
				removed int
			})

			for _, filePatch := range patch.FilePatches() {
				from, to := filePatch.Files()
				var path string
				if to != nil {
					path = to.Path()
				} else if from != nil {
					path = from.Path()
				}
				if path == "" {
					continue
				}

				for _, chunk := range filePatch.Chunks() {
					lines := strings.Count(chunk.Content(), "\n")
					switch chunk.Type() {
					case diff.Add:
						stats[path] = struct {
							added   int
							removed int
						}{
							added:   stats[path].added + lines,
							removed: stats[path].removed,
						}
					case diff.Delete:
						stats[path] = struct {
							added   int
							removed int
						}{
							added:   stats[path].added,
							removed: stats[path].removed + lines,
						}
					}
				}
			}

			// Format stats similar to git diff --stat
			var statLines []string
			var totalAdded, totalRemoved int

			// Get terminal width and calculate layout
			termWidth := getTerminalWidth()
			const (
				minGraphWidth  = 10
				separatorWidth = 5 // space + pipe + space + number + space
			)

			// Find the longest path
			maxPathLen := 0
			for path := range stats {
				if len(path) > maxPathLen {
					maxPathLen = len(path)
				}
			}

			// Calculate graph width with remaining space
			graphWidth := termWidth - maxPathLen - separatorWidth
			if graphWidth < minGraphWidth {
				graphWidth = minGraphWidth
			}

			green := color.New(color.FgGreen).SprintfFunc()
			red := color.New(color.FgRed).SprintfFunc()

			// Format each line
			for path, stat := range stats {
				totalAdded += stat.added
				totalRemoved += stat.removed
				statLines = append(statLines, formatDiffStat(
					path,
					stat.added,
					stat.removed,
					graphWidth,
					func(s string) string { return green("%s", s) },
					func(s string) string { return red("%s", s) },
				))
			}

			if len(statLines) > 0 {
				r.DiffStats = strings.Join(append(statLines, fmt.Sprintf(" %d files changed, %d insertions(+), %d deletions(-)",
					len(stats),
					totalAdded,
					totalRemoved,
				)), "\n")
			}
		}
	}

	return updateErr
}

func (r *Repository) updateOrigin() error {
	// Get current branch
	head, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	auth := r.getAuth()

	// Fetch from origin
	err = r.repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec("+refs/heads/*:refs/remotes/origin/*")},
		Auth:       auth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch from origin: %w", err)
	}

	// Pull changes
	err = w.Pull(&git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: head.Name(),
		Auth:          auth,
	})
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		if err == git.ErrUnstagedChanges {
			return ErrUnstagedChanges
		}
		if err == transport.ErrAuthenticationRequired {
			return fmt.Errorf("authentication required: set GITHUB_TOKEN environment variable for GitHub repositories")
		}
		return fmt.Errorf("failed to pull from origin: %w", err)
	}

	return nil
}

func (r *Repository) updateWithUpstream() error {
	// Get current branch
	head, err := r.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	auth := r.getAuth()

	// Fetch from upstream
	err = r.repo.Fetch(&git.FetchOptions{
		RemoteName: "upstream",
		RefSpecs:   []config.RefSpec{config.RefSpec("+refs/heads/*:refs/remotes/upstream/*")},
		Auth:       auth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch from upstream: %w", err)
	}

	// Get upstream master reference
	upstreamMaster, err := r.repo.Reference("refs/remotes/upstream/master", true)
	if err != nil {
		return fmt.Errorf("failed to get upstream master reference: %w", err)
	}

	// Rebase onto upstream/master
	err = w.Reset(&git.ResetOptions{
		Mode:   git.HardReset,
		Commit: upstreamMaster.Hash(),
	})
	if err != nil {
		return fmt.Errorf("failed to reset to upstream/master: %w", err)
	}

	// Push to origin with force
	err = r.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("%s:refs/heads/%s", head.Name().String(), head.Name().Short()))},
		Force:      true,
		Auth:       auth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		if err == transport.ErrAuthenticationRequired {
			return fmt.Errorf("authentication required: set GITHUB_TOKEN environment variable for GitHub repositories")
		}
		return fmt.Errorf("failed to push to origin: %w", err)
	}

	return nil
}
