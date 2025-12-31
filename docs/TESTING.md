# Testing Documentation

## Overview

This project includes comprehensive testing suite running on GitHub Actions, including:

- **Unit Tests** - Fast, isolated tests for individual functions
- **Integration Tests** - Full stack tests with virtual serial ports (requires socat)
- **Race Detector** - Detects concurrency issues
- **Fuzzing Tests** - Finds edge cases and potential crashes
- **Static Analysis** - Code quality and security checks
- **Benchmark Tests** - Performance regression detection
- **Multi-platform Build Tests** - Ensures code builds on all target platforms

## Test Jobs

### 1. Lint Job

Runs various static analysis tools:
- `gofmt` - Code formatting check
- `go vet` - Go vet static analysis
- `goimports` - Import management
- `golangci-lint` - Comprehensive linter with 20+ rules
- `GoSec` - Security vulnerability scanner

### 2. Test Job (Linux)

Runs full test suite on Ubuntu:
- Race detector enabled
- Coverage profiling
- Benchmark tests
- Integration tests with socat

### 3. Test Windows Job

Runs tests on Windows to ensure cross-platform compatibility.

### 4. Multi-platform Build Job

Builds binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

### 5. Security Job

Security scanning:
- Snyk vulnerability scanner (requires SNYK_TOKEN)
- Go vulnerability checker (govulncheck)

### 6. Benchmark Job (PR only)

Performance regression detection on pull requests:
- Runs benchmarks and compares with baseline
- Fails if performance degrades by >150%

### 7. Fuzz Job

Fuzzing tests to find crashes and edge cases:
- Configuration parser fuzzing
- Listener configuration fuzzing

## Running Tests Locally

### Run all tests
```bash
go test -v ./...
```

### Run tests with race detector
```bash
go test -race ./...
```

### Run tests with coverage
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run fuzz tests
```bash
go test -fuzz=FuzzParseConfig -fuzztime=30s ./...
```

### Run benchmarks
```bash
go test -bench=. -benchmem ./...
```

### Run integration tests (requires socat)
```bash
# Install socat first
sudo apt-get install socat

# Run integration tests
go test -v -run "TestVirtual|TestTCPSerial" ./...
```

## Coverage

Current overall coverage: **10.3%**

### Well-Covered Modules
- `config.go` - 77.8% - 100%
- `listener/queue.go` (RequestCache) - 66.7% - 100%

### Needs More Coverage
- `main.go` - 0% (main entry point, menus, wizard)
- `wizard.go` - 0% (configuration wizard)
- `frp.go` - 0% (FRP client)
- `listener/listener.go` (core logic) - 0% (accept loop, client handling, serial read loop)

## Adding New Tests

### Unit Test Example
```go
func TestMyFunction(t *testing.T) {
    input := "test"
    expected := "result"

    result := MyFunction(input)
    if result != expected {
        t.Errorf("Expected %s, got %s", expected, result)
    }
}
```

### Fuzz Test Example
```go
func FuzzMyParser(f *testing.F) {
    // Add seed corpus
    f.Add([]byte("valid input"))

    f.Fuzz(func(t *testing.T, data []byte) {
        // Should not crash
        _ = MyParser(data)
    })
}
```

## CI/CD Badge

[![Test](https://github.com/whysmx/serial-server/actions/workflows/test.yml/badge.svg)](https://github.com/whysmx/serial-server/actions/workflows/test.yml)

## Required GitHub Secrets

For security scanning, add these secrets to your repository:

- `SNYK_TOKEN` - Optional, for Snyk security scanning
- `CODECOV_TOKEN` - Optional, for Codecov integration

## Troubleshooting

### Integration tests are skipped
Make sure `socat` is installed:
```bash
# Ubuntu/Debian
sudo apt-get install socat

# macOS
brew install socat
```

### golangci-lint fails
Check `.golangci.yml` for linting rules. Some rules can be disabled per file:
```go
//nolint:govet
```

### Coverage upload fails
Check if `CODECOV_TOKEN` is set (optional for public repos).
