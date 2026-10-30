#!/bin/sh
# Renders brand/icons/*.svg to PNG with headless Chrome (transparent corners) and resamples with sips.
# Usage: sh brand/tools/export_icons.sh brand/icons
set -e
DIR=$(cd "${1:-brand/icons}" && pwd)
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
mkdir -p "$DIR/png"
for k in d1 d2 d3; do
  "$CHROME" --headless=new --disable-gpu --hide-scrollbars --default-background-color=00000000 \
    --window-size=1024,1024 --screenshot="$DIR/png/app-icon-$k-1024.png" "file://$DIR/app-icon-$k.svg" >/dev/null 2>&1
  for s in 512 256 128 64 32 16; do
    sips -s format png -z $s $s "$DIR/png/app-icon-$k-1024.png" --out "$DIR/png/app-icon-$k-$s.png" >/dev/null
  done
done
for g in glyph-shield-p glyph-shield-p-light glyph-shield-p-mono; do
  "$CHROME" --headless=new --disable-gpu --hide-scrollbars --default-background-color=00000000 \
    --window-size=256,256 --screenshot="$DIR/png/$g-256.png" "file://$DIR/$g.svg" >/dev/null 2>&1
  for s in 64 32 16; do
    sips -s format png -z $s $s "$DIR/png/$g-256.png" --out "$DIR/png/$g-$s.png" >/dev/null
  done
done
ls "$DIR/png" | wc -l
