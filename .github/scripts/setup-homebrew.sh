#!/bin/sh
# setup-homebrew.sh: make Homebrew available on a macOS or Linux CI runner
# (#124).
#
#   setup-homebrew.sh
#
# The runner may already have Homebrew installed. Otherwise, install it with
# Homebrew's official installer, pinned to a reviewed commit. It must provide
# "brew trust" (Homebrew 6+).

set -eu

# Homebrew/install at a reviewed commit (2026-11-13). Update deliberately.
installer_commit=e5b3c5834e3932c8aaa42deef4794076c9b0cbf0
os=$(uname -s)
brew=$(command -v brew || true)
if [ -z "$brew" ]; then
	case $os in
	Linux) brew=/home/linuxbrew/.linuxbrew/bin/brew ;;
	Darwin)
		case $(uname -m) in
		arm64) brew=/opt/homebrew/bin/brew ;;
		*) brew=/usr/local/bin/brew ;;
		esac
		;;
	*) echo "setup-homebrew: unsupported operating system: $os" >&2; exit 1 ;;
	esac
fi

if [ -x "$brew" ]; then
	echo "setup-homebrew: found the runner's Homebrew: $("$brew" --version | head -n 1)"
else
	url=https://raw.githubusercontent.com/Homebrew/install/$installer_commit/install.sh
	echo "setup-homebrew: installing Homebrew with $url"
	work=$(mktemp -d "${TMPDIR:-/tmp}/setup-homebrew.XXXXXX")
	trap 'rm -rf -- "$work"' EXIT
	curl -fsSL "$url" -o "$work/install.sh"
	NONINTERACTIVE=1 bash "$work/install.sh"
fi

# Homebrew's installer uses the standard prefix for the runner's architecture.
if [ ! -x "$brew" ]; then
	case $os in
	Linux) brew=/home/linuxbrew/.linuxbrew/bin/brew ;;
	Darwin)
		case $(uname -m) in
		arm64) brew=/opt/homebrew/bin/brew ;;
		*) brew=/usr/local/bin/brew ;;
		esac
		;;
	esac
fi
if [ ! -x "$brew" ]; then
	echo "setup-homebrew: Homebrew was not installed at $brew" >&2
	exit 1
fi

# A preinstalled Homebrew can be old: the cask syntax and "brew trust"
# (Homebrew 6+) need a current one.
HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_ENV_HINTS=1 "$brew" update --quiet
echo "setup-homebrew: $("$brew" --version | head -n 1)"
if ! "$brew" commands --quiet | grep -qx trust; then
	echo "setup-homebrew: this Homebrew has no 'brew trust' command (Homebrew 6 or later is required)" >&2
	exit 1
fi

if [ -n "${GITHUB_PATH:-}" ]; then
	prefix=$("$brew" --prefix)
	printf '%s\n' "$prefix/bin" "$prefix/sbin" >>"$GITHUB_PATH"
fi
