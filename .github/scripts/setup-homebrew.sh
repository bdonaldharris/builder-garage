#!/bin/sh
# setup-homebrew.sh: make Homebrew available on a Linux CI runner (#124).
#
#   setup-homebrew.sh
#
# GitHub's Ubuntu runners may already have Homebrew at /home/linuxbrew/.linuxbrew.
# Otherwise it is installed with Homebrew's official installer, pinned to a
# reviewed commit of github.com/Homebrew/install. Either way it is updated,
# and must have "brew trust" (Homebrew 6+). Afterwards, load it with:
#   eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"

set -eu

# Homebrew/install at a reviewed commit (2026-11-13). Update deliberately.
installer_commit=e5b3c5834e3932c8aaa42deef4794076c9b0cbf0
brew=/home/linuxbrew/.linuxbrew/bin/brew

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

# A preinstalled Homebrew can be old: the generated cask's syntax, and
# "brew trust" (Homebrew 6+), need a current one.
HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_ENV_HINTS=1 "$brew" update --quiet
echo "setup-homebrew: $("$brew" --version | head -n 1)"
if ! "$brew" commands --quiet | grep -qx trust; then
	echo "setup-homebrew: this Homebrew has no 'brew trust' command (Homebrew 6 or later is required)" >&2
	exit 1
fi
