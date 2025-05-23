package main

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/trutx/gogitup/internal/config"
	"github.com/trutx/gogitup/internal/git"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan directories for Git repositories",
	Long: `Scan all configured directories for Git repositories.
This command will only scan and list repositories, without updating them.`,
	SilenceUsage: true,
	PreRun: func(cmd *cobra.Command, args []string) {
		if err := viper.BindPFlag("config", cmd.Flags().Lookup("config")); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to bind config flag: %v\n", err)
		}
		if err := viper.BindPFlag("repos-file", cmd.Flags().Lookup("repos-file")); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to bind repos-file flag: %v\n", err)
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Found 0 repositories..."
		s.Start()

		cfg, err := config.LoadConfig()
		if err != nil {
			s.Stop()
			return fmt.Errorf("failed to load config: %w", err)
		}

		repos, err := git.FindRepositories(cfg.Directories, func(count int) {
			s.Suffix = fmt.Sprintf(" Found %d repositories...", count)
		})
		if err != nil {
			s.Stop()
			return fmt.Errorf("failed to find repositories: %w", err)
		}

		// Save repositories to file
		if err := git.SaveRepositories(repos); err != nil {
			s.Stop()
			return fmt.Errorf("failed to save repositories: %w", err)
		}

		s.Stop()
		fmt.Printf("\nFound %d repositories\n", len(repos))

		if verbose {
			fmt.Println("\nRepository list:")
			for _, repo := range repos {
				upstreamStatus := ""
				if repo.HasUpstream {
					upstreamStatus = " (has upstream)"
				}
				fmt.Printf("- %s%s\n", repo.Path, upstreamStatus)
			}
		}

		fmt.Printf("\nResults saved to: %s\n", viper.GetString("repos-file"))
		return nil
	},
}
