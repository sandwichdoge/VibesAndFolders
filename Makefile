# Target executable name
TARGET := VibesAndFolders
GO := go

# --- Standard Go Commands ---

.PHONY: all
all: build

# build: Build for the current (native) OS
# Assumes local Fyne prerequisites (C compiler, etc.) are installed.
.PHONY: build
build:
	@echo "Building for native OS..."
	$(GO) build -o $(TARGET) ./cmd/vibesandfolders

# run: Run the app for the current OS
.PHONY: run
run:
	@echo "Running application..."
	$(GO) run ./cmd/vibesandfolders

# --- Fyne-Cross Cross-Compilation ---
#
# CRITICAL: This project uses Fyne, which requires Cgo.
# Standard 'GOOS=...' cross-compilation will fail.
# You MUST use 'fyne-cross' and have Docker running.
#
# 1. Install tool: make setup
# 2. Run Docker
# 3. Run a build target (e.g., make build-linux)
#

# build-linux: Cross-compile for Linux (amd64)
.PHONY: build-linux
build-linux:
	@echo "Cross-compiling for Linux (amd64)... (Requires Docker)"
	fyne-cross linux ./cmd/vibesandfolders

# build-mac: Cross-compile for macOS (Universal: amd64 + arm64)
.PHONY: build-mac
build-mac:
	@echo "Cross-compiling for macOS Universal Binary... (Requires Docker)"
	fyne-cross darwin ./cmd/vibesandfolders

# build-windows: Cross-compile for Windows (amd64)
.PHONY: build-windows
build-windows:
	@echo "Cross-compiling for Windows (amd64)... (Requires Docker)"
	fyne-cross windows ./cmd/vibesandfolders

# --- Utility Commands ---

# setup: Installs fyne-cross and reminds the user about PATH setup.
.PHONY: setup
setup: install-tools
	@echo "---"
	@echo "✅ Setup Complete: fyne-cross is installed."
	@echo "💡 NOTE: If 'fyne-cross: command not found' occurs, your Go binary path"
	@echo "is not in your shell's PATH environment variable."
	@echo "👉 To fix this PERMANENTLY, add the following line to your ~/.bashrc or ~/.zshrc file:"
	@echo "export PATH=\$PATH:\$(go env GOPATH)/bin"
	@echo "👉 Then run 'source ~/.bashrc' (or ~/.zshrc) or open a new terminal."
	@echo "---"


# install-tools: Installs fyne-cross
.PHONY: install-tools
install-tools:
	@echo "Installing fyne-cross..."
	$(GO) install github.com/fyne-io/fyne-cross@latest
	@echo "Installing fyne..."
# 	$(GO) install fyne.io/fyne/v2/cmd/fyne@latest
	$(GO) install fyne.io/tools/cmd/fyne@latest

# clean: Cleans up build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -f $(TARGET) $(TARGET).exe
	# Remove fyne-cross build directory
	rm -rf ./fyne-cross
	rm -rfv ~/.config/fyne/io.github.sandwichdoge.vibesandfolders
# 	$(GO) clean -cache