#!/bin/bash
# Build the native menu-bar app into Peapod.app.
set -e
cd "$(dirname "$0")"
APP="Peapod.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
swiftc -parse-as-library -O -o "$APP/Contents/MacOS/Peapod" Peapod.swift
cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Peapod</string>
  <key>CFBundleIdentifier</key><string>dev.peapod.ui</string>
  <key>CFBundleExecutable</key><string>Peapod</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
  <key>LSMinimumSystemVersion</key><string>13.0</string>
</dict></plist>
PLIST
echo "built $APP"
echo "run:  PEAPOD_BIN=\"$(cd ../bin 2>/dev/null && pwd)/peapod\" open $APP"
