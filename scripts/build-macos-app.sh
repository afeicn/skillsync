#!/usr/bin/env bash
# Build Skillsync.app (macOS window app: WKWebView + bundled skillsync CLI)
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=${1:-dist}
APP="$OUT/Skillsync.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

go build -ldflags "-s -w -X main.version=${VERSION:-dev}" -o "$APP/Contents/Resources/skillsync" ./cmd/skillsync
swiftc -O app/macos/main.swift -o "$APP/Contents/MacOS/skillsync-app" \
  -framework Cocoa -framework WebKit

if [ ! -f app/macos/AppIcon.png ]; then python3 scripts/make-icon.py; fi
if [ -f app/macos/AppIcon.png ]; then
  ICONSET=$(mktemp -d)/AppIcon.iconset; mkdir -p "$ICONSET"
  for sz in 16 32 64 128 256 512; do
    sips -z $sz $sz app/macos/AppIcon.png --out "$ICONSET/icon_${sz}x${sz}.png" >/dev/null
    dbl=$((sz*2))
    sips -z $dbl $dbl app/macos/AppIcon.png --out "$ICONSET/icon_${sz}x${sz}@2x.png" >/dev/null
  done
  iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/App.icns"
fi

cat > "$APP/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Skillsync</string>
<key>CFBundleDisplayName</key><string>Skillsync</string>
<key>CFBundleExecutable</key><string>skillsync-app</string>
<key>CFBundleIdentifier</key><string>cn.afeicn.skillsync.app</string>
<key>CFBundleIconFile</key><string>App</string>
<key>CFBundleShortVersionString</key><string>0.1.4</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>11.0</string>
</dict></plist>
EOF

codesign --force --deep --sign - "$APP" 2>/dev/null || true
echo "built $APP"
