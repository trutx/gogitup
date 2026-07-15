package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/trutx/gogitup/internal/config"
	"github.com/trutx/gogitup/internal/git"
)

var (
	showStats bool
	threads   int
	noScan    bool
)

type updateResult struct {
	path      string
	error     error
	warning   string
	diffStats string
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().IntVarP(&threads, "threads", "t", runtime.NumCPU(), "number of concurrent repository updates")
	updateCmd.Flags().BoolVarP(&showStats, "stat", "s", false, "show git diff stats for updated repositories")
	updateCmd.Flags().BoolVar(&noScan, "no-scan", false, "disable automatic repository scan before update")
}

// runScan executes the scan command
func runScan() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Found 0 repositories..."
	s.Start()
	defer s.Stop()

	repos, err := git.FindRepositories(cfg.Directories, func(count int) {
		s.Suffix = fmt.Sprintf(" Found %d repositories...", count)
	})
	if err != nil {
		return fmt.Errorf("failed to find repositories: %w", err)
	}

	if err := git.SaveRepositories(repos); err != nil {
		return fmt.Errorf("failed to save repositories: %w", err)
	}

	fmt.Printf("\nFound %d repositories\n", len(repos))
	return nil
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update all found Git repositories",
	Long: `Update all Git repositories by scanning for them and pulling changes.
By default, a scan for new repositories runs automatically before updating.
Disable auto-scan with --no-scan or set auto_scan: false in the config file.

For each repository, it will fetch and pull changes from origin, and for
forks it will rebase onto upstream/master.

Use the -s or --stat flag to show git diff statistics for updated repositories.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRun: func(cmd *cobra.Command, args []string) {
		if err := viper.BindPFlag("config", cmd.Flags().Lookup("config")); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to bind config flag: %v\n", err)
		}
		if err := viper.BindPFlag("repos-file", cmd.Flags().Lookup("repos-file")); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to bind repos-file flag: %v\n", err)
		}

		// Load repositories to validate thread count
		repos, err := git.LoadRepositories()
		if err == nil && len(repos) > 0 {
			// Only validate and adjust thread count if it was explicitly set by the user
			if cmd.Flags().Changed("threads") {
				// Validate and adjust thread count
				if threads < 1 {
					threads = 1
				}
				if threads > len(repos) {
					threads = len(repos)
				}
			}
		} else {
			// If we can't load repositories, just ensure threads is at least 1
			if threads < 1 {
				threads = 1
			}
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Enable verbose mode if stats are requested
		if showStats {
			verbose = true
		}

		// Check repositories file age
		reposFile := viper.GetString("repos-file")
		if reposFile == "" {
			var err error
			reposFile, err = git.GetCacheFile()
			if err != nil {
				return fmt.Errorf("failed to get cache file path: %w", err)
			}
		}

		info, err := os.Stat(reposFile)
		if err == nil {
			age := time.Since(info.ModTime())
			if age > 14*24*time.Hour {
				fmt.Fprintf(os.Stderr, "Warning: Repository list is older than 14 days.\n")
			}
		}

		// Determine if auto-scan should run
		shouldScan := !noScan
		if shouldScan {
			cfg, err := config.LoadConfig()
			if err == nil && cfg.AutoScan != nil && !*cfg.AutoScan {
				shouldScan = false
			}
		}

		if shouldScan {
			if err := runScan(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: auto-scan failed: %v\n", err)
			}
		}

		var s *spinner.Spinner
		if !verbose {
			s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			s.Start()
		}

		// Load repositories from file
		repos, err := git.LoadRepositories()
		if err != nil {
			if s != nil {
				s.Stop()
			}
			return fmt.Errorf("failed to load repositories: %w", err)
		}

		if len(repos) == 0 {
			if s != nil {
				s.Stop()
			}
			return fmt.Errorf("no repositories found. Run 'scan' first")
		}

		// Create work pool
		numWorkers := threads
		if numWorkers < 1 {
			numWorkers = 1
		}
		if numWorkers > len(repos) {
			numWorkers = len(repos)
		}

		// Create channels for work distribution
		jobs := make(chan *git.Repository, len(repos))
		results := make(chan updateResult, len(repos))
		var wg sync.WaitGroup

		// Start worker goroutines
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for repo := range jobs {
					result := updateResult{path: repo.Path}
					err := repo.Update()
					if err != nil {
						if err == git.ErrUncommittedChanges {
							result.warning = "worktree contains uncommitted changes"
						} else {
							result.error = err
						}
					} else {
						result.diffStats = repo.DiffStats
					}
					results <- result
				}
			}()
		}

		// Send work to workers
		for i := range repos {
			jobs <- &repos[i]
		}
		close(jobs)

		// Wait for all updates to complete in a goroutine
		go func() {
			wg.Wait()
			close(results)
		}()

		// Process results as they come in
		count := 0
		errors := make([]error, 0)
		warnings := make(map[string]string)
		for result := range results {
			count++
			if s != nil {
				s.Suffix = fmt.Sprintf(" Updated %d/%d repositories...", count, len(repos))
			} else if verbose {
				fmt.Printf("Progress: %d/%d repositories\n", count, len(repos))
			}

			if result.error != nil {
				errors = append(errors, fmt.Errorf("failed to update %s: %w", result.path, result.error))
				if verbose {
					fmt.Printf("\nError updating %s: %v\n", result.path, result.error)
				}
			} else if result.warning != "" {
				warnings[result.path] = result.warning
				if verbose {
					fmt.Printf("\nWarning: Skipping %s - %s\n", result.path, result.warning)
				}
			} else {
				if verbose {
					fmt.Printf("\nUpdated %s\n", result.path)
				}
				if showStats && result.diffStats != "" {
					fmt.Printf("\nChanges in %s:\n%s\n", result.path, result.diffStats)
				}
			}
		}

		if s != nil {
			s.Stop()
		}
		fmt.Printf("\nUpdated %d repositories\n", len(repos)-len(errors)-len(warnings))

		if len(warnings) > 0 {
			fmt.Printf("\nWarnings for %d repositories:\n", len(warnings))
			for path, warning := range warnings {
				fmt.Printf("- Skipping %s - %s\n", path, warning)
			}
		}

		if len(errors) > 0 {
			fmt.Printf("\nEncountered %d errors:\n", len(errors))
			for _, err := range errors {
				fmt.Printf("- %v\n", err)
			}
			fmt.Printf("\nError: failed to update some repositories\n")
			// Return error code without message since we already printed it
			return fmt.Errorf("")
		}

		return nil
	},
}
