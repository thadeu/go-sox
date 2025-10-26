.PHONY: test bench example clean install-sox

test:
	go test ./... -v -cover

bench:
	go test ./... -bench=. -benchmem -v

example:
	cd examples && go run rtp_to_flac.go

example-sip:
	cd examples && go run sip_integration.go

clean:
	go clean
	rm -f *.test *.out *.tmp *.raw *.flac *.wav *.mp3

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
	golangci-lint run || go vet ./...

fmt:
	go fmt ./...

coverage:
	go test -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

