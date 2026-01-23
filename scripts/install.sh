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
    printf "${GREEN}[INFO]${NC} %s\n" "$1"
}

print_warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

print_error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1" >&2
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

detect_package_manager() {
    if command -v brew &> /dev/null; then
        echo "brew"
    elif command -v apt-get &> /dev/null; then
        echo "apt"
    elif command -v yum &> /dev/null; then
        echo "yum"
    elif command -v pacman &> /dev/null; then
        echo "pacman"
    elif command -v apk &> /dev/null; then
        echo "apk"
    else
        echo "unknown"
    fi
}

install_prerequisites() {
    local pkg_mgr="$1"
    local install_lsp="${2:-false}"

    print_info "Checking prerequisites..."
    
    # Check if ripgrep is installed
    if ! command -v rg &> /dev/null; then
        print_warn "ripgrep not found - required for file search functionality"
        
        case "$pkg_mgr" in
            brew)
                print_info "Installing ripgrep via Homebrew..."
                brew install ripgrep
                ;;
            apt)
                print_info "Installing ripgrep via apt..."
                sudo apt-get update && sudo apt-get install -y ripgrep
                ;;
            yum)
                print_info "Installing ripgrep via yum..."
                sudo yum install -y ripgrep
                ;;
            pacman)
                print_info "Installing ripgrep via pacman..."
                sudo pacman -S --noconfirm ripgrep
                ;;
            apk)
                print_info "Installing ripgrep via apk..."
                sudo apk add --no-cache ripgrep
                ;;
            unknown)
                print_error "Could not detect package manager"
                print_error "Please install ripgrep manually: https://github.com/BurntSushi/ripgrep"
                exit 1
                ;;
        esac
    else
        print_info "✓ ripgrep is already installed"
    fi

    if [ "$install_lsp" = "true" ]; then
        print_info "Installing LSP servers for enhanced IDE features..."
        
        # Install gopls (Go LSP)
        if command -v go &> /dev/null && ! command -v gopls &> /dev/null; then
            print_info "Installing gopls (Go language server)..."
            go install golang.org/x/tools/gopls@latest
        fi

        # Install typescript-language-server (TypeScript/JavaScript)
        if command -v npm &> /dev/null && ! command -v typescript-language-server &> /dev/null; then
            print_info "Installing typescript-language-server..."
            npm install -g typescript-language-server typescript
        fi

        # Install pylsp (Python LSP)
        if command -v pip3 &> /dev/null && ! python3 -c "import pylsp" 2>/dev/null; then
            print_info "Installing python-lsp-server..."
            pip3 install python-lsp-server
        fi

        # Install clangd (C/C++)
        if ! command -v clangd &> /dev/null; then
            case "$pkg_mgr" in
                brew)
                    print_info "Installing clangd via Homebrew..."
                    brew install llvm
                    ;;
                apt)
                    print_info "Installing clangd via apt..."
                    sudo apt-get install -y clangd
                    ;;
                yum)
                    print_info "Installing clangd via yum..."
                    sudo yum install -y clang-tools-extra
                    ;;
                pacman)
                    print_info "Installing clangd via pacman..."
                    sudo pacman -S --noconfirm clang
                    ;;
            esac
        fi
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
        
        # Extract expected checksum for our archive
        local expected_checksum
        expected_checksum=$(grep "$archive_name" checksums.txt | awk '{print $1}')
        
        if [ -z "$expected_checksum" ]; then
            print_warn "Checksum not found for $archive_name, skipping verification"
        else
            # Calculate actual checksum (use shasum on macOS, sha256sum on Linux)
            local actual_checksum
            if command -v sha256sum &> /dev/null; then
                actual_checksum=$(sha256sum "$archive_name" | awk '{print $1}')
            elif command -v shasum &> /dev/null; then
                actual_checksum=$(shasum -a 256 "$archive_name" | awk '{print $1}')
            else
                print_warn "No checksum utility found, skipping verification"
                cd - > /dev/null
                return
            fi
            
            if [ "$expected_checksum" = "$actual_checksum" ]; then
                print_info "Checksum verification passed"
            else
                print_error "Checksum verification failed"
                print_error "Expected: $expected_checksum"
                print_error "Actual:   $actual_checksum"
                exit 1
            fi
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

    # Parse command line arguments
    local install_lsp="false"
    local skip_prereqs="false"
    
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --with-lsp)
                install_lsp="true"
                shift
                ;;
            --skip-prereqs)
                skip_prereqs="true"
                shift
                ;;
            --help)
                echo "Usage: install.sh [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --with-lsp       Install LSP servers (gopls, typescript-language-server, pylsp, clangd)"
                echo "  --skip-prereqs   Skip prerequisite installation (ripgrep, LSP servers)"
                echo "  --help           Show this help message"
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                print_error "Use --help for usage information"
                exit 1
                ;;
        esac
    done

    # Check dependencies
    check_dependencies

    # Detect platform
    local platform
    platform=$(detect_platform)
    print_info "Detected platform: $platform"

    # Detect package manager
    local pkg_mgr
    pkg_mgr=$(detect_package_manager)
    print_info "Detected package manager: $pkg_mgr"
    echo ""

    # Install prerequisites unless skipped
    if [ "$skip_prereqs" != "true" ]; then
        install_prerequisites "$pkg_mgr" "$install_lsp"
        echo ""
    fi

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
    
    if [ "$install_lsp" != "true" ]; then
        echo ""
        print_info "Tip: Run with --with-lsp to install LSP servers for enhanced IDE features"
    fi
}

# Run main function
main "$@"
