#!/bin/sh
# npm-wait-for-packages.sh: wait until npm serves every named package version.
#
#   npm-wait-for-packages.sh VERSION PACKAGE [PACKAGE...]
#
# The bounded retry protects the interval after npm accepts a platform-package
# publish but before its registry replicas serve it. Callers use it before
# publishing a launcher that depends on those packages, and before installing
# a launcher from the registry.

set -eu

if [ "$#" -lt 2 ]; then
	echo "usage: npm-wait-for-packages.sh VERSION PACKAGE [PACKAGE...]" >&2
	exit 2
fi

version=$1
shift

attempts=${GARAGE_NPM_WAIT_ATTEMPTS:-20}
seconds=${GARAGE_NPM_WAIT_SECONDS:-15}
case $attempts in
'' | *[!0-9]*) echo "npm-wait-for-packages: GARAGE_NPM_WAIT_ATTEMPTS must be a positive integer" >&2; exit 2 ;;
esac
case $seconds in
'' | *[!0-9]*) echo "npm-wait-for-packages: GARAGE_NPM_WAIT_SECONDS must be a non-negative integer" >&2; exit 2 ;;
esac
[ "$attempts" -gt 0 ] || {
	echo "npm-wait-for-packages: GARAGE_NPM_WAIT_ATTEMPTS must be a positive integer" >&2
	exit 2
}

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-npm-wait.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

attempt=0
while :; do
	pending=
	for package in "$@"; do
		if npm view "$package@$version" version >"$work/view.out" 2>"$work/view.err"; then
			got=$(cat "$work/view.out")
			[ "$got" = "$version" ] || {
				echo "npm-wait-for-packages: $package@$version returned version '$got'" >&2
				exit 1
			}
		elif grep -q E404 "$work/view.err"; then
			pending="${pending}${pending:+ }$package"
		else
			cat "$work/view.err" >&2
			echo "npm-wait-for-packages: cannot tell whether $package@$version is served" >&2
			exit 1
		fi
	done

	[ -z "$pending" ] && exit 0
	attempt=$((attempt + 1))
	if [ "$attempt" -ge "$attempts" ]; then
		echo "npm-wait-for-packages: timed out waiting for $version: $pending" >&2
		exit 1
	fi
	echo "npm-wait-for-packages: waiting for $version: $pending (attempt $attempt/$attempts)" >&2
	[ "$seconds" -eq 0 ] || sleep "$seconds"
done
