#!/bin/bash
# Build a self-contained Peapod.app (with the peapod CLI bundled inside) and a
# Peapod.dmg installer. Needs the Go toolchain and the Swift toolchain (Xcode CLT).
set -e
cd "$(dirname "$0")"
ROOT="$(cd .. && pwd)"
APP="Peapod.app"

echo "==> building peapod CLI (Go)"
( cd "$ROOT" && go build -o ui-native/peapod-bin ./cmd/peapod )

echo "==> generating icon"
swiftc -O -o /tmp/peapod-iconmaker Icon.swift
rm -rf Peapod.iconset
/tmp/peapod-iconmaker Peapod.iconset
iconutil -c icns Peapod.iconset -o Peapod.icns
rm -rf Peapod.iconset

echo "==> building app (Swift)"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
swiftc -parse-as-library -O -o "$APP/Contents/MacOS/Peapod" Peapod.swift
mv peapod-bin "$APP/Contents/Resources/peapod"
chmod +x "$APP/Contents/Resources/peapod"
cp Peapod.icns "$APP/Contents/Resources/Peapod.icns"
mkdir -p "$APP/Contents/Resources/pt-BR.lproj"
printf '/* Peapod — pt-BR */\n' > "$APP/Contents/Resources/pt-BR.lproj/Localizable.strings"
cp "$ROOT/docs/MANUAL.md" "$APP/Contents/Resources/MANUAL.md"
cp "$ROOT/docs/GUIA.md" "$APP/Contents/Resources/GUIA.md"
cp "$ROOT"/docs/guia-caixa.png "$ROOT"/docs/guia-fluxo.png "$APP/Contents/Resources/" 2>/dev/null || true

cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Peapod</string>
  <key>CFBundleDisplayName</key><string>Peapod</string>
  <key>CFBundleIdentifier</key><string>dev.peapod.ui</string>
  <key>CFBundleExecutable</key><string>Peapod</string>
  <key>CFBundleDevelopmentRegion</key><string>pt-BR</string>
  <key>CFBundleLocalizations</key><array><string>pt-BR</string></array>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>0.1.0</string>
  <key>LSMinimumSystemVersion</key><string>13.0</string>
  <key>CFBundleIconFile</key><string>Peapod</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST

# Bump CFBundleVersion every build so macOS re-renders the icon (avoids stale cache).
BUILDV="$(date +%Y%m%d%H%M%S)"
/usr/libexec/PlistBuddy -c "Add :CFBundleVersion string $BUILDV" "$APP/Contents/Info.plist" 2>/dev/null \
  || /usr/libexec/PlistBuddy -c "Set :CFBundleVersion $BUILDV" "$APP/Contents/Info.plist"

echo "==> signing (ad-hoc)"
codesign --force --deep --sign - "$APP" 2>/dev/null || echo "   (codesign skipped)"

echo "==> building dmg"
STAGE="$(mktemp -d)"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
rm -f Peapod.dmg
hdiutil create -volname "Peapod" -srcfolder "$STAGE" -ov -quiet -format UDZO Peapod.dmg
rm -rf "$STAGE"

echo ""
echo "done:"
echo "  $(pwd)/$APP        — double-click to run"
echo "  $(pwd)/Peapod.dmg  — double-click, then drag Peapod into Applications"
echo "Requires OrbStack (or Docker) running."
