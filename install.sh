#!/bin/sh
# Install the latest gotv-tui binary:
#   curl -fsSL https://raw.githubusercontent.com/dormonbear/gotv-tui/main/install.sh | sh
# Set GOTV_TUI_INSTALL_DIR to install somewhere other than ~/.local/bin.
set -eu

REPO="dormonbear/gotv-tui"
DIR="${GOTV_TUI_INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "gotv-tui: unsupported OS '$os'. On Windows download gotv-tui-windows-x64.exe from https://github.com/$REPO/releases" >&2; exit 1 ;;
esac
case "$arch" in
  x86_64 | amd64) arch=x64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "gotv-tui: unsupported architecture '$arch'" >&2; exit 1 ;;
esac
# A shell running under Rosetta reports x86_64 on Apple Silicon; prefer the native build.
if [ "$os" = darwin ] && [ "$arch" = x64 ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then
  arch=arm64
fi

asset="gotv-tui-$os-$arch"
url="https://github.com/$REPO/releases/latest/download/$asset"

mkdir -p "$DIR"
tmp=$(mktemp "${TMPDIR:-/tmp}/gotv-tui.XXXXXX")
trap 'rm -f "$tmp"' EXIT
echo "Downloading $asset ..."
curl -fSL --progress-bar "$url" -o "$tmp"
chmod 755 "$tmp"
mv "$tmp" "$DIR/gotv-tui"
trap - EXIT

echo "Installed $("$DIR/gotv-tui" --version 2>/dev/null || echo gotv-tui) to $DIR/gotv-tui"
case ":$PATH:" in
  *":$DIR:"*) echo "Run: gotv-tui" ;;
  *) echo "Add $DIR to your PATH, e.g.: echo 'export PATH=\"$DIR:\$PATH\"' >> ~/.zshrc" ;;
esac
