#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
MACPKG_BIN="$REPO_DIR/bin/macpkg"
DIST_DIR="$SCRIPT_DIR/dist"
DESKTOP_DIR="/Users/galaxy/Desktop/sample-app"

echo "=== [1/5] Building macpkg CLI tool ==="
cd "$REPO_DIR"
go build -o "$MACPKG_BIN" ./cmd/macpkg

echo "=== [2/5] Compiling native Cocoa Mach-O binary ==="
cd "$SCRIPT_DIR"
mkdir -p "$DIST_DIR"
clang -framework Cocoa -arch arm64 src/main.m -o "$DIST_DIR/SampleAppBinary"

echo "=== [3/5] Assembling & signing SampleApp.app ==="
"$MACPKG_BIN" app build \
  --name "SampleApp" \
  --id "com.example.sampleapp" \
  --version "1.0.0" \
  --bin "$DIST_DIR/SampleAppBinary" \
  --icon "$SCRIPT_DIR/AppIcon.icns" \
  --out "$DIST_DIR/SampleApp.app"

"$MACPKG_BIN" sign \
  --target "$DIST_DIR/SampleApp.app" \
  --identity "-" \
  --hardened

echo "Validating code signature with native codesign:"
codesign --verify --deep --strict --verbose=2 "$DIST_DIR/SampleApp.app"

echo "=== [4/5] Building Apple Disk Image (SampleApp.dmg) ==="
"$MACPKG_BIN" dmg build \
  --title "SampleApp Installer" \
  --app "$DIST_DIR/SampleApp.app" \
  --out "$DIST_DIR/SampleApp.dmg"

echo "=== [5/5] Building Flat Package Installer (SampleApp.pkg) ==="
"$MACPKG_BIN" pkg build \
  --id "com.example.sampleapp.pkg" \
  --version "1.0.0" \
  --payload "$DIST_DIR/SampleApp.app" \
  --out "$DIST_DIR/SampleApp.pkg"

echo "=== Deploying to Desktop/sample-app for direct double-click testing ==="
mkdir -p "$DESKTOP_DIR"
rm -rf "$DESKTOP_DIR/SampleApp.app" "$DESKTOP_DIR/SampleApp.dmg" "$DESKTOP_DIR/SampleApp.pkg"
cp -R "$DIST_DIR/SampleApp.app" "$DESKTOP_DIR/"
cp "$DIST_DIR/SampleApp.dmg" "$DESKTOP_DIR/"
cp "$DIST_DIR/SampleApp.pkg" "$DESKTOP_DIR/"

echo ""
echo "================================================================="
echo "  SAMPLE APP BUILD COMPLETE & READY FOR DOUBLE-CLICK TESTING!   "
echo "================================================================="
echo "Artifacts created in: $DESKTOP_DIR"
echo "  1. SampleApp.app  (Native Cocoa application bundle)"
echo "  2. SampleApp.dmg  (Apple Disk Image with drag-and-drop installer)"
echo "  3. SampleApp.pkg  (macOS Flat Package installer wizard)"
echo "================================================================="
