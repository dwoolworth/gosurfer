#!/bin/bash
# Create a Chrome profile for gosurfer HumanMode.
#
# This script launches a visible Chrome window with a fresh profile so you can:
#   1. Log into Google, Cloudflare-protected sites, etc.
#   2. Accept cookie banners
#   3. Build browsing history
#
# The profile is saved and can be mounted into containers or used with GOSURFER_PROFILE.
#
# Usage:
#   ./deploy/create-profile.sh [profile-dir]
#
# Default profile location: ~/.gosurfer-profile

set -e

PROFILE_DIR="${1:-$HOME/.gosurfer-profile}"

echo "Creating/updating Chrome profile at: $PROFILE_DIR"
echo "A Chrome window will open. Log into your accounts, then close it."
echo ""

# Find Chrome
if [ -x "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" ]; then
    CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
elif [ -x "/usr/bin/google-chrome" ]; then
    CHROME="/usr/bin/google-chrome"
elif [ -x "/usr/bin/google-chrome-stable" ]; then
    CHROME="/usr/bin/google-chrome-stable"
elif [ -x "/usr/bin/chromium-browser" ]; then
    CHROME="/usr/bin/chromium-browser"
else
    echo "Error: Chrome not found. Set CHROME_BIN or install Chrome."
    exit 1
fi

echo "Using: $CHROME"
echo "Profile: $PROFILE_DIR"
echo ""
echo "Close the browser window when done to save the profile."
echo ""

"$CHROME" \
    --user-data-dir="$PROFILE_DIR" \
    --no-first-run \
    --no-default-browser-check \
    --disable-features=ChromeWhatsNewUI \
    "https://accounts.google.com"

echo ""
echo "Profile saved to: $PROFILE_DIR"
echo ""
echo "Use it with gosurfer:"
echo "  GOSURFER_PROFILE=$PROFILE_DIR GOSURFER_HUMAN=true gosurfer"
echo ""
echo "Copy it to a container:"
echo "  docker cp $PROFILE_DIR container:/home/gosurfer/.chrome-profile"
