#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
VERSION=${PROJECTDOCK_VERSION:-0.10.1}
ARCHIVE=${1:-"$ROOT/outputs/ProjectDock-${VERSION}/ProjectDock-${VERSION}-macos-arm64.zip"}
PKG=${2:-"$ROOT/outputs/ProjectDock-${VERSION}/ProjectDock-${VERSION}-macos-arm64.pkg"}
PROFILE=${PROJECTDOCK_NOTARY_PROFILE:-ProjectDock}
INSTALLER_IDENTITY=${PROJECTDOCK_INSTALLER_IDENTITY:-"Developer ID Installer"}
STAGE=$(mktemp -d "${TMPDIR:-/tmp}/projectdock-pkg.XXXXXX")

trap 'rm -rf "$STAGE"' EXIT

if [ ! -f "$ARCHIVE" ]; then
  echo "找不到已公证的 APP ZIP: $ARCHIVE" >&2
  exit 1
fi

mkdir -p "$(dirname "$PKG")"
unzip -q "$ARCHIVE" -d "$STAGE"
APP="$STAGE/ProjectDock.app"
codesign --verify --deep --strict --verbose=2 "$APP"
xcrun stapler validate "$APP"
spctl -a -vvv -t execute "$APP"

productbuild \
  --component "$APP" /Applications \
  --sign "$INSTALLER_IDENTITY" \
  "$PKG"
pkgutil --check-signature "$PKG"
xcrun notarytool submit "$PKG" --keychain-profile "$PROFILE" --wait
xcrun stapler staple "$PKG"
xcrun stapler validate "$PKG"
spctl -a -vvv -t install "$PKG"
echo "$PKG"
