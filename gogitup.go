package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const (
	defaultThreads        int  = 5
	defaultColors         bool = true
	defaultFollowSymlinks bool = false
)

// Repo holds information about every repo
type Repo struct {
	// Path is the local path where the repo is cloned
	Path string `yaml:"path"`
	// IsFork tells whether the repo was cloned from a fork
	IsFork bool `yaml:"isFork"`
	// UpstreamURL works together with IsFork and specifies the UpstreamURL
	// of the upstream repo where the fork was created from
	UpstreamName string `yaml:"upstreamName"`
}

// Config is where we store all configuration read from the config file
type Config struct {
	Threads        int      `yaml:"threads,omitempty"`
	Colors         bool     `yaml:"colors,omitempty"`
	FollowSymlinks bool     `yaml:"followSymlinks,omitempty"`
	Ignore         []string `yaml:"ignore,omitempty"`
	Repos          []*Repo
}

func (c *Config) expandGlobs() {
	var tmp []*Repo
	for _, repo := range c.Repos {
		if strings.Contains(repo.Path, "*") {
			expanded, err := filepath.Glob(repo.Path)
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
				os.Exit(1)
			}
			for _, e := range expanded {
				tmp = append(tmp, &Repo{e, repo.IsFork, ""})
			}
		} else {
			tmp = append(tmp, repo)
		}
	}
	c.Repos = tmp
}

func (c *Config) removeIgnored() {
	for i, repo := range c.Repos {
		stat, err := os.Stat(repo.Path)
		if err != nil {
			fmt.Printf("ERROR: %v", err)
			os.Exit(1)
		}
		if stat.IsDir() {
			for _, ign := range c.Ignore {
				if repo.Path == ign {
					c.Repos = append(c.Repos[:i], c.Repos[i+1:]...)
				}
			}
		} else {
			for _, ign := range c.Ignore {
				if filepath.Base(repo.Path) == ign {
					c.Repos = append(c.Repos[:i], c.Repos[i+1:]...)
				}
			}
		}
	}
}

func isSymlink(file string) bool {
	f, err := os.Lstat(file)
	if err != nil {
		panic(err)
	}
	if f.Mode()&os.ModeSymlink == 0 {
		return false
	}
	return true
}

func updateRepo(r Repo, c string) ([]uint8, error) {
	out, err := exec.Command("git", "-C", r.Path, "-c", c, "pull").CombinedOutput()
	if err != nil {
		return out, err
	}
	return out, nil
}

func updateFork(r Repo, c string) ([]uint8, error) {

	if r.UpstreamName == "" {
		r.UpstreamName = "upstream"
	}

	// git fetch UpstreamName
	output, err := exec.Command("git", "-C", r.Path, "-c", c, "fetch", r.UpstreamName).CombinedOutput()
	if err != nil {
		return output, err
	}

	// git rebase UpstreamName/master
	out, err := exec.Command("git", "-C", r.Path, "-c", c, "rebase", r.UpstreamName+"/master").CombinedOutput()
	output = append(output, out...)
	if err != nil {
		return output, err
	}

	// git push
	out, err = exec.Command("git", "-C", r.Path, "-c", c, "push").CombinedOutput()
	output = append(output, out...)
	if err != nil {
		return output, err
	}

	return output, nil

}

// Receives work from the channel
func worker(repoCh chan Repo, outCh chan string, id int, colors, symlinks bool) {
	var out string
	var err error
	var result []uint8
	var colorParam string

	if colors {
		colorParam = "color.ui=always"
	} else {
		colorParam = "color.ui=never"
	}

	for {

		r := <-repoCh

		if !symlinks && isSymlink(r.Path) {
			out = fmt.Sprintf("+++ '%s' is a symlink, ignoring... (thread #%d)\n", r.Path, id)
		} else {
			out = fmt.Sprintf("+++ Now working with '%s' (thread #%d):\n", r.Path, id)

			if r.IsFork {
				result, err = updateFork(r, colorParam)
			} else {
				result, err = updateRepo(r, colorParam)
			}
			out = out + string(result)
			if err != nil {
				out = out + "+++ Found some errors, but continuing...\n"
			}
		}
		out = out + "---------------------------------\n"
		outCh <- out

	}

}

// Sends work down the workers channel
func scheduler(r Repo, repoCh chan Repo) {
	repoCh <- r
}

func main() {
	userName, _ := user.Current()
	homeDir := userName.HomeDir

	// Option parser
	configFile := flag.String("c", homeDir+"/.gogitup.yaml", "Configuration file")
	flag.Parse()

	// Read config file contents
	c, err := ioutil.ReadFile(*configFile)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	// Set defaults and parse the YAML we just read
	conf := Config{}
	conf.Threads = defaultThreads
	conf.Colors = defaultColors
	conf.FollowSymlinks = defaultFollowSymlinks

	if err := yaml.Unmarshal(c, &conf); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	for _, repo := range conf.Repos {
		if repo.Path[:2] == "~/" {
			repo.Path = strings.Replace(repo.Path, "~/", homeDir+"/", 1)
		}
	}
	for i, ign := range conf.Ignore {
		if ign[:2] == "~/" {
			conf.Ignore[i] = strings.Replace(ign, "~/", homeDir+"/", 1)
		}
	}

	conf.expandGlobs()

	if len(conf.Ignore) > 0 {
		conf.removeIgnored()
	}

	repoCh := make(chan Repo)
	outCh := make(chan string)

	// Spawn as many workers as specified in the config file
	for id := 1; id <= conf.Threads; id++ {
		go worker(repoCh, outCh, id, conf.Colors, conf.FollowSymlinks)
	}

	// Schedule as many repo updates as repos are configured in the config file
	for _, repo := range conf.Repos {
		go scheduler(*repo, repoCh)
	}

	// Print results
	for j := 0; j < len(conf.Repos); j++ {
		fmt.Printf("%s", <-outCh)
	}

}
