#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
VERSION=${PROJECTDOCK_VERSION:-0.10.1}
ARCHIVE=${1:-"$ROOT/outputs/ProjectDock-${VERSION}/ProjectDock-${VERSION}-macos-arm64.zip"}
PROFILE=${PROJECTDOCK_NOTARY_PROFILE:-ProjectDock}
STAGE=$(mktemp -d "${TMPDIR:-/tmp}/projectdock-notary.XXXXXX")

trap 'rm -rf "$STAGE"' EXIT

if [ ! -f "$ARCHIVE" ]; then
  echo "找不到待公证制品: $ARCHIVE" >&2
  exit 1
fi

xcrun notarytool history --keychain-profile "$PROFILE" >/dev/null
xcrun notarytool submit "$ARCHIVE" --keychain-profile "$PROFILE" --wait
unzip -q "$ARCHIVE" -d "$STAGE"
xcrun stapler staple "$STAGE/ProjectDock.app"
xcrun stapler validate "$STAGE/ProjectDock.app"
codesign --verify --deep --strict --verbose=2 "$STAGE/ProjectDock.app"
spctl -a -vvv -t execute "$STAGE/ProjectDock.app"
NOTARIZED_ARCHIVE="$STAGE/ProjectDock-${VERSION}-notarized.zip"
(
  cd "$STAGE"
  COPYFILE_DISABLE=1 zip -qryX "$NOTARIZED_ARCHIVE" ProjectDock.app
)
mv "$NOTARIZED_ARCHIVE" "$ARCHIVE"
echo "$ARCHIVE"
