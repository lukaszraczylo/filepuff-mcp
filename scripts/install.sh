#!/bin/bash

# MCP Filepuff Installation Script
# Downloads and installs the latest version of mcp-filepuff

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# GitHub repository information
REPO="lukaszraczylo/filepuff-mcp"
BINARY_NAME="mcp-filepuff"

# Installation directory (will try ~/.local/bin first, then /usr/local/bin)
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Functions
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

detect_platform() {
    local os
    local arch

    # Detect OS
    case "$(uname -s)" in
        Linux*)     os="linux" ;;
        Darwin*)    os="darwin" ;;
        MINGW*|MSYS*|CYGWIN*)
            print_error "Windows is not directly supported. Please use WSL2."
            exit 1
            ;;
        *)
            print_error "Unsupported operating system: $(uname -s)"
            exit 1
            ;;
    esac

    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64)   arch="amd64" ;;
        arm64|aarch64)  arch="arm64" ;;
        *)
            print_error "Unsupported architecture: $(uname -m)"
            exit 1
            ;;
    esac

    # Warn about unsupported combinations
    if [ "$os" = "linux" ] && [ "$arch" = "arm64" ]; then
        print_error "Linux ARM64 builds are not currently available due to CGO cross-compilation complexity"
        print_error "Please build from source: https://github.com/lukaszraczylo/filepuff-mcp#build-from-source"
        exit 1
    fi

    echo "${os}_${arch}"
}

check_dependencies() {
    local missing_deps=()

    # Check for required commands
    for cmd in curl tar; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done

    if [ ${#missing_deps[@]} -ne 0 ]; then
        print_error "Missing required dependencies: ${missing_deps[*]}"
        print_error "Please install them and try again."
        exit 1
    fi
}

get_latest_version() {
    local version
    version=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)

    if [ -z "$version" ]; then
        print_error "Failed to fetch latest version from GitHub"
        exit 1
    fi

    echo "$version"
}

download_and_install() {
    local version="$1"
    local platform="$2"
    local tmpdir

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    print_info "Downloading mcp-filepuff ${version} for ${platform}..."

    # Construct download URLs
    local archive_name="mcp-filepuff_${version#v}_${platform}.tar.gz"
    local archive_url="https://github.com/${REPO}/releases/download/${version}/${archive_name}"
    local checksum_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"

    # Download archive
    if ! curl -sSfL "$archive_url" -o "$tmpdir/$archive_name"; then
        print_error "Failed to download $archive_url"
        exit 1
    fi

    # Download checksums
    if ! curl -sSfL "$checksum_url" -o "$tmpdir/checksums.txt"; then
        print_warn "Failed to download checksums, skipping verification"
    else
        print_info "Verifying checksum..."
        cd "$tmpdir"
        if grep "$archive_name" checksums.txt | sha256sum -c --status; then
            print_info "Checksum verification passed"
        else
            print_error "Checksum verification failed"
            exit 1
        fi
        cd - > /dev/null
    fi

    # Extract archive
    print_info "Extracting archive..."
    tar -xzf "$tmpdir/$archive_name" -C "$tmpdir"

    # Ensure install directory exists
    if [ ! -d "$INSTALL_DIR" ]; then
        print_info "Creating installation directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi

    # Install binary
    print_info "Installing to $INSTALL_DIR/$BINARY_NAME..."
    if [ -w "$INSTALL_DIR" ]; then
        cp "$tmpdir/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
        chmod +x "$INSTALL_DIR/$BINARY_NAME"
    else
        print_warn "No write permission to $INSTALL_DIR, trying with sudo..."
        sudo cp "$tmpdir/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
        sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"
    fi

    print_info "Installation complete!"
}

check_path() {
    if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
        print_warn "$INSTALL_DIR is not in your PATH"
        print_warn "Add it to your PATH by adding this line to your shell profile:"
        echo ""
        echo "    export PATH=\"\$PATH:$INSTALL_DIR\""
        echo ""
    fi
}

verify_installation() {
    if command -v "$BINARY_NAME" &> /dev/null; then
        local installed_version
        installed_version=$("$BINARY_NAME" -version 2>&1 || echo "unknown")
        print_info "Successfully installed: $installed_version"
    else
        print_warn "Binary installed but not found in PATH"
        print_info "You can run it directly: $INSTALL_DIR/$BINARY_NAME"
    fi
}

main() {
    print_info "MCP Filepuff Installation Script"
    echo ""

    # Check dependencies
    check_dependencies

    # Detect platform
    local platform
    platform=$(detect_platform)
    print_info "Detected platform: $platform"

    # Get latest version
    local version
    version=$(get_latest_version)
    print_info "Latest version: $version"
    echo ""

    # Download and install
    download_and_install "$version" "$platform"
    echo ""

    # Check PATH
    check_path

    # Verify installation
    verify_installation
    echo ""

    print_info "To get started, run: $BINARY_NAME --help"
}

# Run main function
main "$@"
