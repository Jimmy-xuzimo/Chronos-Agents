#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
arch="${1:-$(go env GOARCH)}"
case "$arch" in amd64|arm64) ;; *) echo 'Supported: amd64, arm64' >&2; exit 2;; esac
test "$arch" = "$(go env GOARCH)" || { echo "Build each architecture on a matching macOS runner" >&2; exit 2; }
test "$(uname -s)" = Darwin || { echo 'Build on macOS with Xcode tools' >&2; exit 2; }
app="artifacts/macos-$arch/Chronos Agent.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
MACOSX_DEPLOYMENT_TARGET=13.0 CGO_ENABLED=1 GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$app/Contents/MacOS/chronos-agent" ./cmd/agent
swift_arch="$arch"; if test "$arch" = amd64; then swift_arch=x86_64; fi
swiftc -O -target "$swift_arch-apple-macosx13.0" -framework Security -o "$app/Contents/MacOS/chronos-keychain" deploy/macos/Keychain.swift
cat > "$app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.chronos.agents</string>
<key>CFBundleName</key><string>Chronos Agent</string>
<key>CFBundleExecutable</key><string>chronos-agent</string>
<key>CFBundleShortVersionString</key><string>0.1.0</string>
<key>CFBundleVersion</key><string>1</string>
<key>LSUIElement</key><true/>
<key>CFBundleIconFile</key><string>Agent.icns</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
</dict></plist>
PLIST
iconset="artifacts/macos-$arch/Agent.iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" internal/desktop/icon.png --out "$iconset/icon_$size"x"$size.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" internal/desktop/icon.png --out "$iconset/icon_$size"x"$size@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/Agent.icns"
cp LICENSE NOTICE THIRD_PARTY_NOTICES.md "$app/Contents/Resources/"
if test -d artifacts/third-party; then cp -R artifacts/third-party "$app/Contents/Resources/third-party"; fi
if test -f artifacts/bom.cdx.json; then cp artifacts/bom.cdx.json "$app/Contents/Resources/"; fi
echo "Built unsigned application: $app"
