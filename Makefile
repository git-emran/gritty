.PHONY: run build build-mac clean

# Run directly (for development)
run:
	go run ./cmd/term

# Build raw binary
build:
	go build -o term ./cmd/term

# Build macOS .app bundle — produces Gritty.app in the project root
build-mac:
	$(shell go env GOPATH)/bin/fyne package -os darwin -src ./cmd/term -name Gritty -icon $(shell pwd)/assets/icon.png

# Remove build artifacts
clean:
	rm -rf term Gritty.app
