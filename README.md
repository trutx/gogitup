# GoGitUp

GoGitUp is a command-line tool that helps you manage and update multiple Git repositories efficiently. It scans configured directories for Git repositories and can update them all with a single command, handling both origin and upstream remotes.

## ⚠️ Code Quality Warning

**This codebase was generated using AI-assisted development (Cursor and Claude Code) and has been functionally reviewed only.** This means that if it works, it's considered good enough. However, you should expect:

- Inconsistencies in coding style and patterns
- Superfluous or redundant code
- Suboptimal implementations
- Other code quality issues typical of AI-generated code

The focus was on functionality rather than code elegance. Use at your own discretion.

*(This warning was also AI-generated lol)*

## Features

- 🔍 Scan directories recursively for Git repositories
- 🔄 Update multiple repositories in parallel
- ⚡ Fast repository discovery and caching
- 🔒 GitHub token support for private repositories
- 🔱 Support for fork workflow (origin/upstream remotes)
- 📦 Native Git LFS support for large file repositories
- 💾 Cache repository information for faster subsequent runs
- 🚫 Skip repositories with uncommitted changes to tracked files

## Installation

### Prerequisites

- Go 1.26.5 or later (matching `go.mod`)
- Git (gogitup runs `git` for every operation, so your existing git setup applies)
- Git LFS (optional, required only for LFS repositories)

### From Source

```bash
go install github.com/trutx/gogitup/cmd/gogitup@latest
```

### Manual Build

```bash
git clone https://github.com/trutx/gogitup.git
cd gogitup
make build      # Build the binary
make install    # Install to $GOPATH/bin
```

## Development

The project includes several Make targets to help with development:

```bash
make build      # Build the binary
make install    # Build and install to $GOPATH/bin
make test       # Run tests with race detection and coverage
make coverage   # Generate and view test coverage report
make lint       # Run Go vet
make clean      # Remove build artifacts and coverage files
make all        # Clean and build
```

## Configuration

Create a configuration file at `~/.gogitup.yaml` (or use `--config` flag):

```yaml
directories:
  - ~/code/projects
  - ~/repos
  - /path/to/repositories

# Optional: disable automatic scanning before updates (default: true)
# auto_scan: false
```

For GitHub private repositories, set your GitHub token:

```bash
export GITHUB_TOKEN=your_token_here
```

The token is offered to git as a fallback credential helper for `github.com` only,
and only behind whatever helper you already configured.

### Git configuration

GoGitUp delegates fetching, merging and pushing to `git`, so everything in your
git configuration is honoured automatically:

- `url.*.insteadOf` rewriting (e.g. SSH to HTTPS)
- `credential.helper` and other credential sources
- `include` / `includeIf` files, `http.*` settings and proxies
- `core.sshCommand` and `GIT_*` environment variables

If `git fetch` works in a repository, `gogitup update` works there too.

## Usage

### Scan for Repositories

```bash
# Scan configured directories
gogitup scan

# Show verbose output
gogitup scan -v
```

### Update Repositories

```bash
# Update all repositories (automatically scans for new repos first)
gogitup update

# Skip automatic scan before updating
gogitup update --no-scan

# Show status information during update
gogitup update --stat

# Show verbose output
gogitup update -v

# Limit how many repositories are updated concurrently (default: number of CPUs)
gogitup update --threads 4
```

#### Git LFS Support

GoGitUp detects repositories that use Git Large File Storage (LFS) from their `.gitattributes` and checks that `git-lfs` is installed before updating them. The update itself is the same `git fetch` every other repository gets, so LFS smudge/filter handling is exactly what git does.

Note: Git LFS must be installed on your system to handle LFS repositories.

### Cache Management

Repository information is cached by default in:
- Linux: `~/.cache/gogitup/repositories.json`
- macOS: `~/Library/Caches/gogitup/repositories.json`
- Windows: `%LocalAppData%\gogitup\repositories.json`

You can specify a custom cache location with the `--repos-file` flag.

## Error Handling

GoGitUp handles various error scenarios:
- Repositories with uncommitted changes to tracked files are skipped
- Fetch, merge and push follow git's own behaviour, so authentication, proxy and
  `insteadOf` problems surface with git's own message
- Directories that cannot be read are skipped during scan; per-repository
  failures during update are reported with the path and git's message

Two broken-repository cases come with repair instructions instead of a bare git
error:
- `bad object refs/...` / `did not send all necessary objects`: cached
  remote-tracking refs point at commits the clone never fetched
- `unresolved deltas left after unpacking`: the object store itself is
  incomplete (common in shallow clones touched by old gogitup releases)

Both hints name the exact `git` commands to run; GoGitUp never repairs a
repository on its own.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see below for details:

```
MIT License

Copyright (c) 2025 Roger Torrentsgenerós

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
``` 
