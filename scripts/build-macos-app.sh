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

if [ -f cmd/skillsync-tray/icon.png ]; then
  sips -s format icns cmd/skillsync-tray/icon.png --out "$APP/Contents/Resources/App.icns" >/dev/null || true
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
