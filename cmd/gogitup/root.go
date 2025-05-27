package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/trutx/gogitup/internal/git"
)

var (
	configFile string
	reposFile  string
	verbose    bool
	rootCmd    = &cobra.Command{
		Use:   "gogitup",
		Short: "A tool to automatically update Git repositories",
		Long: `gogitup is a CLI tool that automatically updates Git repositories in specified directories.
It can handle both regular repositories and forks with upstream remotes.`,
	}
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error getting user home directory:", err)
		os.Exit(1)
	}
	defaultConfig := fmt.Sprintf("%s/.gogitup.yaml", home)

	// Get default repos file from git package
	defaultReposFile, err := git.GetCacheFile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error getting default repos file:", err)
		os.Exit(1)
	}

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", defaultConfig, "config file path")
	rootCmd.PersistentFlags().StringVarP(&reposFile, "repos-file", "r", defaultReposFile, "repository list file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "show verbose output")
	rootCmd.AddCommand(scanCmd)
}
