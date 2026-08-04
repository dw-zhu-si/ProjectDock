#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
DESTINATION=${1:-"$ROOT/work/native-070"}
EMBEDDED_PROJECTCTL="$ROOT/work/build/projectctl-darwin-arm64"
PROJECTCTL=${PROJECTCTL_BINARY:-"$EMBEDDED_PROJECTCTL"}
DERIVED_DATA=${PROJECTDOCK_DERIVED_DATA:-"${TMPDIR:-/tmp}/ProjectDockDerivedData-0100"}
SIGNING_IDENTITY=${PROJECTDOCK_SIGNING_IDENTITY:-"Developer ID Application"}
STAGE=$(mktemp -d "${TMPDIR:-/tmp}/projectdock-app.XXXXXX")
BUILD_ROOT="$STAGE/source"
APP_SOURCE="$DERIVED_DATA/Build/Products/Release/ProjectDock.app"
APP="$STAGE/ProjectDock.app"

trap 'rm -rf "$STAGE"' EXIT

mkdir -p "$DESTINATION"
DESTINATION=$(CDPATH= cd -- "$DESTINATION" && pwd)
ARCHIVE="$DESTINATION/ProjectDock-0.10.0-macos-arm64.zip"
mkdir -p "$(dirname "$EMBEDDED_PROJECTCTL")"
if [ -z "${PROJECTCTL_BINARY:-}" ]; then
  (cd "$ROOT" && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$EMBEDDED_PROJECTCTL" ./cmd/projectctl)
  PROJECTCTL="$EMBEDDED_PROJECTCTL"
fi
if [ ! -x "$PROJECTCTL" ]; then
  echo "缺少可执行文件: $PROJECTCTL" >&2
  exit 1
fi
if [ "$("$PROJECTCTL" version)" != "projectctl 0.10.0" ]; then
  echo "待嵌入的 projectctl 版本不是 0.10.0" >&2
  exit 1
fi
if [ "$PROJECTCTL" != "$EMBEDDED_PROJECTCTL" ]; then
  ditto "$PROJECTCTL" "$EMBEDDED_PROJECTCTL"
fi
chmod +x "$EMBEDDED_PROJECTCTL"
xattr -cr "$EMBEDDED_PROJECTCTL"
codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$EMBEDDED_PROJECTCTL"

mkdir -p "$BUILD_ROOT/Shared" "$BUILD_ROOT/work/build"
ditto "$ROOT/ProjectDock.xcodeproj" "$BUILD_ROOT/ProjectDock.xcodeproj"
ditto "$ROOT/native" "$BUILD_ROOT/native"
ditto "$ROOT/ProjectDockWidgetExtension" "$BUILD_ROOT/ProjectDockWidgetExtension"
ditto "$ROOT/Shared/ProjectDockWidgetShared060.swift" "$BUILD_ROOT/Shared/ProjectDockWidgetShared060.swift"
ditto "$EMBEDDED_PROJECTCTL" "$BUILD_ROOT/work/build/projectctl-darwin-arm64"

xcodebuild \
  -project "$BUILD_ROOT/ProjectDock.xcodeproj" \
  -scheme ProjectDock \
  -configuration Release \
  -derivedDataPath "$DERIVED_DATA" \
  -destination "platform=macOS,arch=arm64" \
  CODE_SIGNING_ALLOWED=NO \
  clean build

if [ "$(shasum -a 256 "$APP_SOURCE/Contents/Resources/projectctl-darwin-arm64" | awk '{print $1}')" != "$(shasum -a 256 "$EMBEDDED_PROJECTCTL" | awk '{print $1}')" ]; then
  echo "APP 内 projectctl 与已验证二进制不一致" >&2
  exit 1
fi

xattr -cr "$APP_SOURCE"
codesign \
  --force \
  --options runtime \
  --timestamp \
  --entitlements "$ROOT/ProjectDockWidgetExtension/ProjectDockWidgetExtension.entitlements" \
  --sign "$SIGNING_IDENTITY" \
  "$APP_SOURCE/Contents/PlugIns/ProjectDockWidgetExtension.appex"
xattr -cr "$APP_SOURCE"
codesign \
  --force \
  --options runtime \
  --timestamp \
  --entitlements "$ROOT/native/ProjectDock.entitlements" \
  --sign "$SIGNING_IDENTITY" \
  "$APP_SOURCE"

ditto "$APP_SOURCE" "$APP"
xattr -cr "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
(
  cd "$STAGE"
  COPYFILE_DISABLE=1 zip -qryX "$ARCHIVE" ProjectDock.app
)
if zipinfo -1 "$ARCHIVE" | grep -Eq '^__MACOSX/|/\._|^\._'; then
  echo "ZIP 含有 AppleDouble 扩展属性条目" >&2
  exit 1
fi
mkdir -p "$STAGE/verify"
unzip -q "$ARCHIVE" -d "$STAGE/verify"
codesign --verify --deep --strict --verbose=2 "$STAGE/verify/ProjectDock.app"
echo "$ARCHIVE"
