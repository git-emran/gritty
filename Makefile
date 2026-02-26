.PHONY: run build build-mac clean

# Run directly (for development)
run:
	go run ./cmd/term

# Build raw binary
build:
	CGO_CFLAGS="-Wno-deprecated-declarations" go build -o term ./cmd/term

# Build macOS .app bundle — produces Gritty.app in the project root
build-mac: build
	mkdir -p Gritty.app/Contents/MacOS
	mkdir -p Gritty.app/Contents/Resources
	cp term Gritty.app/Contents/MacOS/Gritty
	@echo '<?xml version="1.0" encoding="UTF-8"?>' > Gritty.app/Contents/Info.plist
	@echo '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' >> Gritty.app/Contents/Info.plist
	@echo '<plist version="1.0">' >> Gritty.app/Contents/Info.plist
	@echo '<dict>' >> Gritty.app/Contents/Info.plist
	@echo '    <key>CFBundleExecutable</key><string>Gritty</string>' >> Gritty.app/Contents/Info.plist
	@echo '    <key>CFBundleIconFile</key><string>icon.png</string>' >> Gritty.app/Contents/Info.plist
	@echo '    <key>CFBundleIdentifier</key><string>com.emranhossain.gritty</string>' >> Gritty.app/Contents/Info.plist
	@echo '    <key>CFBundleName</key><string>Gritty</string>' >> Gritty.app/Contents/Info.plist
	@echo '    <key>CFBundlePackageType</key><string>APPL</string>' >> Gritty.app/Contents/Info.plist
	@echo '    <key>CFBundleShortVersionString</key><string>1.0.0</string>' >> Gritty.app/Contents/Info.plist
	@echo '</dict>' >> Gritty.app/Contents/Info.plist
	@echo '</plist>' >> Gritty.app/Contents/Info.plist
	cp assets/icon.png Gritty.app/Contents/Resources/icon.png

# Remove build artifacts
clean:
	rm -rf term Gritty.app
