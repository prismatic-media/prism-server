#!/bin/bash
set -e

REPO="prismatic-media/prism-server"
INSTALL_DIR="/opt/prism"
VERSION_FILE="${INSTALL_DIR}/.version"
BINARY_PATH="${INSTALL_DIR}/prism"

# Check if curl and jq are installed
if ! command -v curl &> /dev/null || ! command -v jq &> /dev/null; then
    echo "Warning: curl or jq not installed. Skipping auto-update check."
    exit 0
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        ASSET_NAME="prism-linux-amd64"
        ;;
    aarch64|arm64)
        ASSET_NAME="prism-linux-arm64"
        ;;
    *)
        echo "Warning: Unsupported architecture $ARCH. Skipping auto-update."
        exit 0
        ;;
esac

echo "Checking GitHub for the latest release of ${REPO}..."
RELEASE_JSON=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest")

# Check if we got a valid release JSON
LATEST_TAG=$(echo "$RELEASE_JSON" | jq -r '.tag_name // empty')
if [ -z "$LATEST_TAG" ]; then
    echo "Warning: Could not fetch latest release tag (rate limit or network issue). Skipping update."
    exit 0
fi

CURRENT_VERSION=""
if [ -f "$VERSION_FILE" ]; then
    CURRENT_VERSION=$(cat "$VERSION_FILE")
fi

if [ "$CURRENT_VERSION" = "$LATEST_TAG" ] && [ -f "$BINARY_PATH" ]; then
    echo "Prism is up-to-date (Version: $CURRENT_VERSION)."
    exit 0
fi

echo "New version available: ${LATEST_TAG} (Current: ${CURRENT_VERSION:-none})"

DOWNLOAD_URL=$(echo "$RELEASE_JSON" | jq -r ".assets[] | select(.name == \"${ASSET_NAME}\") | .browser_download_url")

if [ -z "$DOWNLOAD_URL" ] || [ "$DOWNLOAD_URL" = "null" ]; then
    echo "Warning: Could not find download URL for asset ${ASSET_NAME}. Skipping update."
    exit 0
fi

echo "Downloading ${ASSET_NAME} from ${DOWNLOAD_URL}..."
TEMP_BINARY=$(mktemp)
if ! curl -sL -o "$TEMP_BINARY" "$DOWNLOAD_URL"; then
    echo "Warning: Failed to download update. Skipping update."
    rm -f "$TEMP_BINARY"
    exit 0
fi

# Make it executable
chmod +x "$TEMP_BINARY"

# Backup current binary if it exists
if [ -f "$BINARY_PATH" ]; then
    mv "$BINARY_PATH" "${BINARY_PATH}.bak"
fi

# Install new binary
mkdir -p "$INSTALL_DIR"
if mv "$TEMP_BINARY" "$BINARY_PATH"; then
    echo "$LATEST_TAG" > "$VERSION_FILE"
    echo "Successfully updated Prism to ${LATEST_TAG}."
    rm -f "${BINARY_PATH}.bak"
else
    echo "Error: Failed to install new binary. Restoring backup..."
    if [ -f "${BINARY_PATH}.bak" ]; then
        mv "${BINARY_PATH}.bak" "$BINARY_PATH"
    fi
    exit 0
fi
