#!/bin/bash
# Install script for Prism Media Server
set -e

# Ensure the script is run as root
if [ "$EUID" -ne 0 ]; then
    echo "Error: Please run this script as root (e.g. using sudo)."
    exit 1
fi

# Determine local directories (if run from a cloned repository)
# Using fallback if running via piped stdin (e.g. curl ... | sh)
if [ -n "${BASH_SOURCE[0]}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    REPO_ROOT="$(dirname "$SCRIPT_DIR")"
else
    SCRIPT_DIR=""
    REPO_ROOT=""
fi

# GitHub URL configurations for standalone install
GITHUB_REPO="prismatic-media/prism-server"
BRANCH="main"
RAW_BASE_URL="https://raw.githubusercontent.com/${GITHUB_REPO}/${BRANCH}"
SERVICE_URL="${RAW_BASE_URL}/prism.service"
UPDATE_URL="${RAW_BASE_URL}/scripts/update.sh"

# Paths to install
INSTALL_DIR="/opt/prism"
DATA_DIR="/var/lib/prism"
SERVICE_DEST="/etc/systemd/system/prism.service"
UPDATE_DEST="${INSTALL_DIR}/update.sh"

echo "=== Prism Installation ==="

# 1. Create prism group and user if they do not exist
echo "Setting up system user and group 'prism'..."
if ! getent group prism >/dev/null; then
    groupadd -r prism
    echo "Group 'prism' created."
fi

if ! getent passwd prism >/dev/null; then
    useradd -r -g prism -d "$INSTALL_DIR" -s /usr/sbin/nologin -c "Prism Media Server" prism
    echo "User 'prism' created."
fi

# 2. Create installation and database directories
echo "Creating directories..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$DATA_DIR"

# 3. Copy or download service and update scripts
echo "Installing files..."

# Check if curl is available
if ! command -v curl &> /dev/null; then
    echo "Error: curl is required to run the installer."
    exit 1
fi

# Install prism.service
if [ -n "$REPO_ROOT" ] && [ -f "${REPO_ROOT}/prism.service" ]; then
    echo "Installing local prism.service..."
    cp "${REPO_ROOT}/prism.service" "$SERVICE_DEST"
else
    echo "Downloading prism.service from GitHub..."
    if ! curl -sL -o "$SERVICE_DEST" "$SERVICE_URL"; then
        echo "Error: Failed to download systemd service file from ${SERVICE_URL}"
        exit 1
    fi
fi
chmod 644 "$SERVICE_DEST"
echo "Installed systemd service to ${SERVICE_DEST}."

# Install update.sh
if [ -n "$SCRIPT_DIR" ] && [ -f "${SCRIPT_DIR}/update.sh" ]; then
    echo "Installing local update.sh..."
    cp "${SCRIPT_DIR}/update.sh" "$UPDATE_DEST"
else
    echo "Downloading update.sh from GitHub..."
    if ! curl -sL -o "$UPDATE_DEST" "$UPDATE_URL"; then
        echo "Error: Failed to download update script from ${UPDATE_URL}"
        exit 1
    fi
fi
chmod 750 "$UPDATE_DEST"
chown prism:prism "$UPDATE_DEST"
echo "Installed update script to ${UPDATE_DEST}."

# 4. Set ownership and permissions
echo "Applying directory permissions..."
chown -R prism:prism "$INSTALL_DIR"
chown -R prism:prism "$DATA_DIR"
chmod 750 "$INSTALL_DIR"
chmod 750 "$DATA_DIR"

# 5. Reload systemd daemon
echo "Reloading systemd..."
systemctl daemon-reload

# 6. Perform initial update to download the binary
echo "Running initial update to fetch latest Prism release..."
# Run as the prism user so that files are created with correct ownership
if sudo -u prism "$UPDATE_DEST"; then
    echo "Initial update check complete."
else
    echo "Warning: Initial update check failed, but installation will continue."
fi

echo "=== Prism Installation Complete ==="
echo ""
echo "To start the Prism service now:"
echo "  sudo systemctl start prism"
echo ""
echo "To enable the Prism service to start at boot:"
echo "  sudo systemctl enable prism"
echo ""
echo "To check the service status:"
echo "  sudo systemctl status prism"
echo ""
