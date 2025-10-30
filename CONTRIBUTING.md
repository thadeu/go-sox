# Contributing to go-sox

Thank you for your interest in contributing to go-sox! This document provides guidelines and instructions for contributing.

## Development Setup

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/go-sox.git`
3. Install SoX (required for tests):
   ```bash
   make install-sox
   ```
4. Install Go dependencies:
   ```bash
   go mod download
   ```

## Code Style

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Run `make fmt` before committing
- Run `make lint` to check code quality
- Ensure all tests pass: `make test`
- Ensure benchmarks don't regress: `make bench`

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run specific test suite
go test -v -run TestConverterSuite

# Run benchmarks
make bench
```

### Writing Tests

- Use `testify` suite for test organization
- Test both success and failure cases
- Include edge cases (empty input, timeouts, cancellations)
- Verify resource cleanup in streaming tests

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/) format:

- `feat: Add new feature`
- `fix: Bug fix`
- `docs: Documentation changes`
- `refactor: Code refactoring`
- `test: Test changes`
- `perf: Performance improvements`
- `chore: Maintenance tasks`

Examples:
```
feat: Add support for custom SoX effects
fix: Handle empty input in converter
docs: Update README with performance benchmarks
```

## Pull Request Process

1. Ensure your code follows the style guidelines
2. Add tests for new functionality
3. Update documentation if needed
4. Ensure all tests pass
5. Update CHANGELOG.md with your changes
6. Create a pull request with a clear description

## Code Review

- All PRs require review before merging
- Address review comments promptly
- Be respectful and constructive in discussions

## Questions?

Open an issue for questions or discussions about the project.

