package git

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// Common errors
var (
	ErrUncommittedChanges = fmt.Errorf("worktree contains uncommitted changes to tracked files")
	ErrGitNotInstalled    = errors.New("git is not installed or not available in PATH")
)

// Repository represents a Git repository
type Repository struct {
	Path        string    `json:"path"`
	HasUpstream bool      `json:"has_upstream"`
	LastScanned time.Time `json:"last_scanned"`
	DiffStats   string    `json:"-"`
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

	// Filter out repositories that are no longer valid
	validRepos := make([]Repository, 0, len(repositories))
	for _, repository := range repositories {
		if _, err := remoteNames(repository.Path); err == nil {
			validRepos = append(validRepos, repository)
		}
	}

	return validRepos, nil
}

// FindRepositories searches for Git repositories in the given directories
func FindRepositories(directories []string, onFound func(count int)) ([]Repository, error) {
	var repositories []Repository
	count := 0

	for _, dir := range directories {

		// Expand home directory if path starts with ~/
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

			if !hasGitDir(path) {
				return nil
			}

			// Asking git for the remotes both confirms the repository is usable
			// and tells us whether it is a fork
			remotes, err := remoteNames(path)
			if err != nil {
				return nil
			}

			repositories = append(repositories, Repository{
				Path:        path,
				HasUpstream: slices.Contains(remotes, "upstream"),
			})
			count++
			if onFound != nil {
				onFound(count)
			}

			return filepath.SkipDir
		})

		if err != nil {
			return nil, fmt.Errorf("failed to walk directory %s: %w", dir, err)
		}
	}

	return repositories, nil
}

// hasGitDir reports whether path contains a .git directory or gitfile.
func hasGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

// remoteNames returns the configured remotes of a repository. An error means
// git cannot work with path as a repository.
func remoteNames(path string) ([]string, error) {
	out, err := gitOutput(path, "remote")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
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

// isLFSRepository checks if the repository uses Git LFS by looking for .gitattributes with LFS entries
func (r *Repository) isLFSRepository() bool {
	// Check .gitattributes file
	gitattributes := filepath.Join(r.Path, ".gitattributes")
	data, err := os.ReadFile(gitattributes)
	if err != nil {
		return false
	}

	// Look for LFS patterns (e.g., "*.bin filter=lfs diff=lfs merge=lfs")
	return strings.Contains(string(data), "filter=lfs")
}

// Update updates the repository by fetching and pulling changes
func (r *Repository) Update() error {
	if err := ensureGitAvailable(); err != nil {
		return err
	}

	if r.isLFSRepository() {
		if _, err := exec.LookPath("git-lfs"); err != nil {
			return fmt.Errorf("git-lfs is not installed: %w", err)
		}
	}

	// Repositories without tracked files have nothing to update
	tracked, err := gitOutput(r.Path, "ls-files")
	if err != nil {
		return err
	}
	if tracked == "" {
		return nil
	}

	if err := r.checkCleanWorktree(); err != nil {
		return err
	}

	oldHead, err := gitOutput(r.Path, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current HEAD: %w", err)
	}

	if err := r.fetchAndFastForward(); err != nil {
		return err
	}

	newHead, err := gitOutput(r.Path, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get new HEAD: %w", err)
	}

	// Only report stats when the update actually moved HEAD
	if newHead == oldHead {
		return nil
	}

	stats, err := r.diffStats(oldHead)
	if err != nil {
		return err
	}
	r.DiffStats = stats

	return nil
}

// checkCleanWorktree returns ErrUncommittedChanges when tracked files have
// staged or unstaged modifications. Untracked files are ignored.
func (r *Repository) checkCleanWorktree() error {
	staged, err := gitSucceeded(r.Path, "diff-index", "--quiet", "HEAD", "--")
	if err != nil {
		return fmt.Errorf("failed to check staged changes: %w", err)
	}
	if !staged {
		return ErrUncommittedChanges
	}

	unstaged, err := gitSucceeded(r.Path, "diff-files", "--quiet", "--")
	if err != nil {
		return fmt.Errorf("failed to check unstaged changes: %w", err)
	}
	if !unstaged {
		return ErrUncommittedChanges
	}

	return nil
}

// fetchAndFastForward fetches the repository's remotes and fast-forwards the
// current branch, pushing to origin afterwards when the repository is a fork.
func (r *Repository) fetchAndFastForward() error {
	if r.HasUpstream {
		if err := gitRun(r.Path, "fetch", "upstream"); err != nil {
			return fmt.Errorf("failed to fetch from upstream: %w", r.withFetchHint(err))
		}
	} else {
		if err := gitRun(r.Path, "fetch", "origin"); err != nil {
			return fmt.Errorf("failed to fetch from origin: %w", r.withFetchHint(err))
		}
	}

	branch, err := currentBranch(r.Path)
	if err != nil {
		return err
	}

	remote := "origin"
	if r.HasUpstream {
		remote = "upstream"
	}

	if err := r.fastForward(remote, branch); err != nil {
		return err
	}

	// Keep the fork's origin in sync with upstream
	if r.HasUpstream {
		if err := gitRun(r.Path, "push", "origin", branch); err != nil {
			return fmt.Errorf("failed to push to origin: %w", err)
		}
	}

	return nil
}

// withFetchHint adds repair instructions when a fetch failed because the local
// object store is incomplete or its cached refs point at commits it does not
// have. go-git used to record a ref for every advertised branch, which left such
// dangling refs behind, most often in shallow clones.
func (r *Repository) withFetchHint(err error) error {
	msg := err.Error()
	quoted := fmt.Sprintf("%q", r.Path)

	// Deltas against objects that were never stored: the object database itself
	// is incomplete, so ref pruning alone cannot help
	if strings.Contains(msg, "unresolved deltas left after unpacking") ||
		strings.Contains(msg, "unpack-objects failed") {
		hint := "hint: the object store is incomplete, so this clone cannot fetch; repair with:\n" +
			fmt.Sprintf("  git -C %s fetch --refetch %sorigin", quoted, r.unshallowFlag())
		return fmt.Errorf("%w\n%s", err, hint)
	}

	staleRefs := strings.Contains(msg, "did not send all necessary objects") ||
		strings.Contains(msg, "does not point to a valid object") ||
		strings.Contains(msg, "bad object refs/")
	if !staleRefs {
		return err
	}

	hint := "hint: cached remote-tracking refs point at commits this clone does not have; repair with:\n" +
		fmt.Sprintf("  git -C %s for-each-ref --format='delete %%(refname)' refs/remotes/origin | grep -v 'refs/remotes/origin/HEAD' | git -C %s update-ref --stdin\n", quoted, quoted) +
		fmt.Sprintf("  git -C %s fetch --prune origin", quoted)

	if r.unshallowFlag() != "" {
		hint += fmt.Sprintf("\n  (shallow clone: 'git -C %s fetch --unshallow origin' also restores full history)", quoted)
	}

	return fmt.Errorf("%w\n%s", err, hint)
}

// unshallowFlag returns "--unshallow " for shallow clones and an empty string
// otherwise.
func (r *Repository) unshallowFlag() string {
	out, err := gitOutput(r.Path, "rev-parse", "--is-shallow-repository")
	if err == nil && out == "true" {
		return "--unshallow "
	}
	return ""
}

// fastForward merges remote/branch, refusing anything but a fast-forward.
func (r *Repository) fastForward(remote, branch string) error {
	target := remote + "/" + branch

	err := gitRun(r.Path, "merge", "--ff-only", target)
	if err == nil {
		return nil
	}

	merged, ancestorErr := gitSucceeded(r.Path, "merge-base", "--is-ancestor", target, "HEAD")
	if ancestorErr == nil && !merged {
		return fmt.Errorf("cannot fast-forward to %s: the local branch has diverged from it. Resolve manually (consider rebasing or merging)", target)
	}

	return fmt.Errorf("failed to merge %s: %w", target, err)
}

// currentBranch returns the short name of the checked out branch.
func currentBranch(path string) (string, error) {
	branch, err := gitOutput(path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch == "" {
		return "", fmt.Errorf("failed to determine current branch: HEAD is detached or unborn")
	}
	return branch, nil
}

// diffStats returns a git diff --stat rendering of the changes between two commits.
func (r *Repository) diffStats(oldHead string) (string, error) {
	width := fmt.Sprintf("--stat=%d", getTerminalWidth())

	colorMode := "--color=always"
	if color.NoColor {
		colorMode = "--color=never"
	}

	out, err := gitRawOutput(r.Path, "diff", width, colorMode, oldHead+"..HEAD")
	if err == nil {
		return out, nil
	}

	// Fall back to showing the commit itself, which also covers updates that
	// only advanced HEAD without a common ancestor
	shown, showErr := gitRawOutput(r.Path, "show", width, colorMode, "HEAD")
	if showErr != nil {
		return "", fmt.Errorf("failed to get diff stats: %w", err)
	}

	return shown, nil
}

// ensureGitAvailable verifies that git can be executed.
func ensureGitAvailable() error {
	if _, err := exec.LookPath("git"); err != nil {
		return ErrGitNotInstalled
	}
	return nil
}

// credentialArgs makes GITHUB_TOKEN available to git for github.com only, as a
// fallback behind whatever credential helper the user configured. Git applies
// the URL-scoped helper itself, so no other host is ever offered the token.
func credentialArgs() []string {
	if os.Getenv("GITHUB_TOKEN") == "" {
		return nil
	}

	const helper = `!f() { echo username=x-access-token; echo "password=$GITHUB_TOKEN"; }; f`
	return []string{"-c", "credential.https://github.com.helper=" + helper}
}

// gitCommand builds a git command that runs inside dir.
func gitCommand(dir string, args ...string) *exec.Cmd {
	full := append(credentialArgs(), args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	return cmd
}

// runGitExec executes git and returns its standard output, exit code and
// standard error. A negative exit code reports that git could not be executed.
func runGitExec(dir string, args ...string) (stdout string, exitCode int, stderr string) {
	cmd := gitCommand(dir, args...)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	out, err := cmd.Output()
	exitCode = 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return string(out), exitCode, strings.TrimSpace(errBuf.String())
}

// gitOutput runs git and returns its trimmed standard output.
func gitOutput(dir string, args ...string) (string, error) {
	out, exitCode, stderr := runGitExec(dir, args...)
	if exitCode != 0 {
		return "", gitError(args, exitCode, stderr)
	}
	return strings.TrimSpace(out), nil
}

// gitRawOutput runs git and returns its standard output without trimming.
func gitRawOutput(dir string, args ...string) (string, error) {
	out, exitCode, stderr := runGitExec(dir, args...)
	if exitCode != 0 {
		return "", gitError(args, exitCode, stderr)
	}
	return strings.TrimRight(out, "\n"), nil
}

// gitRun runs git and discards its output unless the command fails.
func gitRun(dir string, args ...string) error {
	_, exitCode, stderr := runGitExec(dir, args...)
	if exitCode != 0 {
		return gitError(args, exitCode, stderr)
	}
	return nil
}

// gitSucceeded reports whether git exited with status 0. Commands in git use
// status 1 to answer "no", so that is not treated as a failure.
func gitSucceeded(dir string, args ...string) (bool, error) {
	_, exitCode, stderr := runGitExec(dir, args...)
	switch exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, gitError(args, exitCode, stderr)
	}
}

// gitError builds an error that carries git's own message.
func gitError(args []string, exitCode int, stderr string) error {
	if stderr == "" {
		return fmt.Errorf("git %s failed with exit code %d", strings.Join(args, " "), exitCode)
	}
	return errors.New(stderr)
}
