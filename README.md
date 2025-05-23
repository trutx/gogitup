# GoGitUp

GoGitUp is a command-line tool that helps you manage and update multiple Git repositories efficiently. It scans configured directories for Git repositories and can update them all with a single command, handling both origin and upstream remotes.

## Features

- 🔍 Scan directories recursively for Git repositories
- 🔄 Update multiple repositories in parallel
- ⚡ Fast repository discovery and caching
- 🔒 GitHub token support for private repositories
- 🔱 Support for fork workflow (origin/upstream remotes)
- 💾 Cache repository information for faster subsequent runs
- 🚫 Skip repositories with unstaged changes

## Installation

### From Source

Requires Go 1.21 or later.

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
```

For GitHub private repositories, set your GitHub token:

```bash
export GITHUB_TOKEN=your_token_here
```

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
# Update all repositories
gogitup update

# Show status information during update
gogitup update --stat

# Show verbose output
gogitup update -v
```

### Cache Management

Repository information is cached by default in:
- Linux: `~/.cache/gogitup/repositories.json`
- macOS: `~/Library/Caches/gogitup/repositories.json`
- Windows: `%LocalAppData%\gogitup\repositories.json`

You can specify a custom cache location with the `--repos-file` flag.

## Error Handling

GoGitUp handles various error scenarios:
- Repositories with unstaged changes are skipped
- Authentication errors for private repositories
- Invalid or corrupted Git repositories
- Inaccessible directories or files

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
