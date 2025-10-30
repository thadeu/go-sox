# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for receiving such patches depends on the CVSS v3.0 Rating:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it via email to the maintainer rather than creating a public issue.

**Do not** create a public GitHub issue for security vulnerabilities.

### Reporting Process

1. Email the maintainer with details about the vulnerability
2. Include steps to reproduce the issue
3. Include potential impact assessment
4. We will acknowledge receipt within 48 hours
5. We will provide an estimated timeline for a fix
6. Once fixed, we will credit you in the release notes (if desired)

### Security Considerations

When using go-sox in production:

- **SoX Binary**: Ensure SoX is installed from trusted sources
- **Input Validation**: Validate audio input before conversion
- **Resource Limits**: Set appropriate ulimits for file descriptors and processes
- **Context Timeouts**: Always use context timeouts for conversions
- **Circuit Breaker**: Configure circuit breaker appropriately for your use case

### Known Security Considerations

- go-sox executes the SoX binary as a subprocess. Ensure:
  - SoX binary path is validated if using custom paths
  - Input/output paths are validated to prevent path traversal
  - Users have appropriate file system permissions

## Security Updates

Security updates will be released as patch versions (e.g., 1.0.0 → 1.0.1).

Subscribe to GitHub releases to be notified of security updates.

