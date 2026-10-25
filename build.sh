#!/usr/bin/env bash
# Builds the Windows plugin and packs it into dist/AppVolume-OpenDeck-plugin.zip
set -euo pipefail
cd "$(dirname "$0")"
go run ./tools/genimg plugin/com.rihardo.appvolume.sdPlugin/imgs
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui" \
  -o plugin/com.rihardo.appvolume.sdPlugin/appvolume.exe .
mkdir -p dist
rm -f dist/AppVolume-OpenDeck-plugin.zip
(cd plugin && zip -qr ../dist/AppVolume-OpenDeck-plugin.zip com.rihardo.appvolume.sdPlugin)
echo "Built dist/AppVolume-OpenDeck-plugin.zip"
