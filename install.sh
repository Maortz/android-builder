#!/usr/bin/env bash
set -euo pipefail

REPO="Maortz/android-builder"
BINARY="builder"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported arch: $ARCH"; exit 1 ;;
esac

VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')
[ -z "$VERSION" ] && { echo "Could not get latest version"; exit 1; }

URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}_${OS}_${ARCH}"
echo "Installing android-builder ${VERSION} (${OS}/${ARCH})..."
curl -fsSL "$URL" -o "/tmp/${BINARY}"
chmod +x "/tmp/${BINARY}"

if [ -w "$INSTALL_DIR" ]; then
    mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
    sudo mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed: ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Next steps:"
echo "  builder auth github       # save GitHub token"
echo "  builder init              # add android-build.yml + update builder.json"
echo "  builder android build     # trigger GHA build, download APK"
echo "  builder dev flutter       # install APK + hot-reload session"
