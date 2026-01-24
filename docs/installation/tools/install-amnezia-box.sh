#!/bin/sh
set -e

# amnezia-box installer
# Usage: curl -fsSL https://raw.githubusercontent.com/hoaxisr/amnezia-box/main/docs/installation/tools/install-amnezia-box.sh | sh
# Or with options: curl -fsSL ... | sh -s -- --prerelease --systemd

REPO="hoaxisr/amnezia-box"
BINARY_NAME="sing-box"

# Installation paths
INSTALL_BIN="/usr/local/bin"
INSTALL_CONFIG="/usr/local/etc/sing-box"
INSTALL_DATA="/var/lib/sing-box"
SYSTEMD_PATH="/etc/systemd/system"

# Colors (if terminal supports it)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

info() { printf "${BLUE}[INFO]${NC} %s\n" "$1"; }
success() { printf "${GREEN}[OK]${NC} %s\n" "$1"; }
warn() { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }
error() { printf "${RED}[ERROR]${NC} %s\n" "$1" >&2; exit 1; }

# Privilege escalation helper
run_privileged() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    elif command -v sudo >/dev/null 2>&1; then
        sudo "$@"
    elif command -v doas >/dev/null 2>&1; then
        doas "$@"
    else
        error "Root privileges required. Install sudo or run as root."
    fi
}

# Parse arguments
download_prerelease=false
download_version=""
install_systemd=auto
do_uninstall=false

print_help() {
    cat << 'EOF'
amnezia-box installer

Usage: install-amnezia-box.sh [OPTIONS]

Installation options:
  --prerelease       Install latest pre-release version
  --version VER      Install specific version (e.g., 1.12.17-awg2.0)
  --systemd          Install systemd service (default on systemd systems)
  --no-systemd       Skip systemd service installation

Other options:
  --uninstall        Remove amnezia-box and all its files
  --help             Show this help

Examples:
  # Install latest stable
  curl -fsSL https://raw.githubusercontent.com/hoaxisr/amnezia-box/main/docs/installation/tools/install-amnezia-box.sh | sh

  # Install with systemd service
  curl -fsSL ... | sh -s -- --systemd

  # Install pre-release
  curl -fsSL ... | sh -s -- --prerelease

  # Uninstall
  curl -fsSL ... | sh -s -- --uninstall
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --prerelease|--beta|--alpha)
            download_prerelease=true
            shift
            ;;
        --version)
            shift
            if [ $# -eq 0 ]; then
                error "Missing argument for --version"
            fi
            download_version="$1"
            shift
            ;;
        --systemd)
            install_systemd=yes
            shift
            ;;
        --no-systemd)
            install_systemd=no
            shift
            ;;
        --uninstall)
            do_uninstall=true
            shift
            ;;
        --help|-h)
            print_help
            exit 0
            ;;
        *)
            error "Unknown argument: $1. Use --help for usage."
            ;;
    esac
done

# Detect architecture
detect_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)
            echo "linux-amd64"
            ;;
        aarch64|arm64)
            echo "entware-aarch64"
            ;;
        mips|mipsel)
            echo "entware-mipsel"
            ;;
        *)
            error "Unsupported architecture: $arch"
            ;;
    esac
}

# Detect OS
detect_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux)
            echo "linux"
            ;;
        darwin)
            error "macOS is not currently supported. Build from source."
            ;;
        *)
            error "Unsupported OS: $os"
            ;;
    esac
}

# Check if systemd is available
has_systemd() {
    command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]
}

# Check required tools
check_requirements() {
    for cmd in curl; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            error "Required tool not found: $cmd"
        fi
    done
}

# Get latest version from GitHub API
get_latest_version() {
    api_url="https://api.github.com/repos/${REPO}/releases"

    if [ "$download_prerelease" = "true" ]; then
        api_url="${api_url}?per_page=1"
    else
        api_url="${api_url}/latest"
    fi

    if [ -n "$GITHUB_TOKEN" ]; then
        response=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" "$api_url")
    else
        response=$(curl -fsSL "$api_url")
    fi

    if [ $? -ne 0 ]; then
        error "Failed to fetch release info from GitHub"
    fi

    if [ "$download_prerelease" = "true" ]; then
        tag=$(echo "$response" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"$/\1/')
    else
        tag=$(echo "$response" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/')
    fi

    if [ -z "$tag" ]; then
        error "Could not parse release version from GitHub response"
    fi

    echo "$tag" | sed 's/^v//'
}

# Generate systemd unit file
generate_systemd_unit() {
    cat << 'EOF'
[Unit]
Description=amnezia-box service (sing-box with AWG support)
Documentation=https://sing-box.sagernet.org
After=network.target nss-lookup.target network-online.target

[Service]
Type=simple
User=sing-box
Group=sing-box
StateDirectory=sing-box
WorkingDirectory=/var/lib/sing-box
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SYS_PTRACE CAP_DAC_READ_SEARCH
ExecStart=/usr/local/bin/sing-box -D /var/lib/sing-box -C /usr/local/etc/sing-box run
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=10s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
EOF
}

# Generate default config
generate_default_config() {
    cat << 'EOF'
{
  "log": {
    "level": "info",
    "timestamp": true
  },
  "dns": {
    "servers": [
      {
        "tag": "google",
        "address": "tls://8.8.8.8"
      }
    ]
  },
  "inbounds": [],
  "outbounds": [
    {
      "type": "direct",
      "tag": "direct"
    }
  ]
}
EOF
}

# Setup systemd service
setup_systemd() {
    info "Setting up systemd service..."

    # Create sing-box user if not exists
    if ! id sing-box >/dev/null 2>&1; then
        info "Creating sing-box user..."
        run_privileged useradd -r -M -s /usr/sbin/nologin sing-box || true
    fi

    # Create directories
    run_privileged mkdir -p "$INSTALL_CONFIG"
    run_privileged mkdir -p "$INSTALL_DATA"
    run_privileged chown sing-box:sing-box "$INSTALL_DATA"

    # Create default config if not exists
    if [ ! -f "$INSTALL_CONFIG/config.json" ]; then
        info "Creating default configuration..."
        generate_default_config | run_privileged tee "$INSTALL_CONFIG/config.json" >/dev/null
        run_privileged chmod 640 "$INSTALL_CONFIG/config.json"
        run_privileged chown root:sing-box "$INSTALL_CONFIG/config.json"
    else
        info "Configuration already exists, skipping..."
    fi

    # Install systemd unit
    generate_systemd_unit | run_privileged tee "$SYSTEMD_PATH/sing-box.service" >/dev/null
    run_privileged systemctl daemon-reload

    success "Systemd service installed"
    info "Edit config: $INSTALL_CONFIG/config.json"
    info "Then run: sudo systemctl enable --now sing-box"
}

# Uninstall
do_uninstall() {
    info "Uninstalling amnezia-box..."

    # Stop and disable service
    if has_systemd; then
        if systemctl is-active --quiet sing-box 2>/dev/null; then
            info "Stopping sing-box service..."
            run_privileged systemctl stop sing-box
        fi

        if systemctl is-enabled --quiet sing-box 2>/dev/null; then
            info "Disabling sing-box service..."
            run_privileged systemctl disable sing-box
        fi

        if [ -f "$SYSTEMD_PATH/sing-box.service" ]; then
            info "Removing systemd unit..."
            run_privileged rm -f "$SYSTEMD_PATH/sing-box.service"
            run_privileged systemctl daemon-reload
        fi
    fi

    # Remove files
    if [ -f "$INSTALL_BIN/$BINARY_NAME" ]; then
        info "Removing binary..."
        run_privileged rm -f "$INSTALL_BIN/$BINARY_NAME"
    fi

    if [ -d "$INSTALL_DATA" ]; then
        info "Removing data directory..."
        run_privileged rm -rf "$INSTALL_DATA"
    fi

    # Ask about config
    if [ -d "$INSTALL_CONFIG" ]; then
        warn "Configuration directory exists: $INSTALL_CONFIG"
        printf "Remove configuration? [y/N] "
        read -r answer
        case "$answer" in
            [Yy]*)
                run_privileged rm -rf "$INSTALL_CONFIG"
                success "Configuration removed"
                ;;
            *)
                info "Configuration preserved"
                ;;
        esac
    fi

    # Remove user (optional)
    if id sing-box >/dev/null 2>&1; then
        printf "Remove sing-box user? [y/N] "
        read -r answer
        case "$answer" in
            [Yy]*)
                run_privileged userdel sing-box 2>/dev/null || true
                success "User removed"
                ;;
            *)
                info "User preserved"
                ;;
        esac
    fi

    echo ""
    success "Uninstallation complete!"
}

# Main installation
install_binary() {
    info "amnezia-box installer"
    info "Repository: https://github.com/${REPO}"
    echo ""

    check_requirements
    detect_os >/dev/null

    arch_name=$(detect_arch)
    info "Detected architecture: $arch_name"

    # Determine version
    if [ -z "$download_version" ]; then
        info "Fetching latest version..."
        download_version=$(get_latest_version)
    fi
    info "Version: $download_version"

    # Build download URL
    asset_name="sing-box-${download_version}-${arch_name}"
    download_url="https://github.com/${REPO}/releases/download/v${download_version}/${asset_name}"

    info "Downloading from: $download_url"

    # Create temp directory
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    # Download binary
    if [ -n "$GITHUB_TOKEN" ]; then
        curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" -o "${tmp_dir}/${BINARY_NAME}" "$download_url"
    else
        curl -fsSL -o "${tmp_dir}/${BINARY_NAME}" "$download_url"
    fi

    if [ $? -ne 0 ]; then
        error "Failed to download binary"
    fi

    success "Downloaded successfully"

    # Make executable
    chmod +x "${tmp_dir}/${BINARY_NAME}"

    # Stop existing service if running
    if has_systemd && systemctl is-active --quiet sing-box 2>/dev/null; then
        info "Stopping existing service..."
        run_privileged systemctl stop sing-box
    fi

    # Install binary
    info "Installing to ${INSTALL_BIN}/${BINARY_NAME}..."
    run_privileged mv "${tmp_dir}/${BINARY_NAME}" "${INSTALL_BIN}/${BINARY_NAME}"

    # Set capabilities for TUN interface creation and privileged ports
    if command -v setcap >/dev/null 2>&1; then
        info "Setting capabilities for TUN support..."
        run_privileged setcap 'cap_net_admin,cap_net_bind_service=+ep' "${INSTALL_BIN}/${BINARY_NAME}"
    else
        warn "setcap not found - you may need to run as root for TUN interfaces"
    fi

    success "Installed to ${INSTALL_BIN}/${BINARY_NAME}"

    # Verify installation
    if command -v sing-box >/dev/null 2>&1; then
        installed_version=$(sing-box version 2>/dev/null | head -1 || echo "unknown")
        success "Verification: ${installed_version}"
    fi

    # Setup systemd if requested
    if [ "$install_systemd" = "yes" ]; then
        if has_systemd; then
            echo ""
            setup_systemd
        else
            warn "Systemd not available, skipping service setup"
        fi
    elif [ "$install_systemd" = "auto" ] && has_systemd; then
        echo ""
        info "To install systemd service, re-run with --systemd flag"
    fi

    # Restart service if it was running
    if has_systemd && systemctl is-enabled --quiet sing-box 2>/dev/null; then
        info "Restarting service..."
        run_privileged systemctl start sing-box
        success "Service restarted"
    fi

    echo ""
    success "Installation complete!"
    info "Run 'sing-box help' to get started"
    info "Documentation: https://sing-box.sagernet.org"
}

# Entry point
main() {
    if [ "$do_uninstall" = "true" ]; then
        do_uninstall
    else
        install_binary
    fi
}

main "$@"
