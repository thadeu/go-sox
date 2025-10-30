.PHONY: test bench example clean install-sox lint vet fmt check test-deps check-deps security version

test:
	gotestsum --format=short-verbose

bench:
	go test ./... -bench=. -benchmem -v

example:
	cd examples && go run rtp_to_flac.go

example-sip:
	cd examples && go run sip_integration.go

clean:
	go clean
	rm -f *.test *.out *.tmp *.raw *.flac *.wav *.mp3
	rm -f coverage.out coverage.html

install-sox:
	@echo "Installing SoX..."
	@if [ "$$(uname)" = "Darwin" ]; then \
		brew install sox; \
	elif [ "$$(uname)" = "Linux" ]; then \
		if command -v apt-get >/dev/null 2>&1; then \
			sudo apt-get install -y sox; \
		elif command -v yum >/dev/null 2>&1; then \
			sudo yum install -y sox; \
		fi; \
	fi

check-sox:
	@sox --version || (echo "SoX not installed. Run 'make install-sox'" && exit 1)

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, falling back to go vet"; \
		go vet ./...; \
	fi

vet:
	go vet ./...

fmt:
	go fmt ./...

check-deps:
	@echo "Verifying dependencies..."
	go mod verify
	go mod tidy -v

security:
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
	fi

coverage:
	go test -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

check: fmt lint vet test
	@echo "All checks passed"

version:
	@echo "go-sox version: $(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"
	@echo "Go version: $(shell go version)"
	@echo "SoX version: $(shell sox --version 2>/dev/null | head -n1 || echo 'not installed')"

