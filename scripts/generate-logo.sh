#!/bin/bash
#
# Need `imagemagick`
# Temporary using it by `nix-shell -p imagemagick`
#

INPUT="logo.png"

echo "Generating PNG files (forcing RGBA)..."
magick "$INPUT" -resize 32x32 -alpha set -background none -strip 32x32.png
magick "$INPUT" -resize 128x128 -alpha set -background none -strip 128x128.png
magick "$INPUT" -resize 256x256 -alpha set -background none -strip 128x128@2x.png

echo "Generating icon.ico..."
magick "$INPUT" -resize 256x256 \
  -define icon:auto-resize="256,128,64,48,32,16" \
  -compress zip icon.ico

echo "Generating icon.icns (optimized version)..."
magick "$INPUT" -resize 1024x1024 \
  -define icon:auto-resize="16,32,64,128,256,512,1024" \
  -compress zip -strip icon.icns

echo "Done!"
