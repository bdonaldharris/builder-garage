#!/bin/sh
# npm-dist-tag.sh: choose the npm dist-tag for publishing a garage release
# (#158).
#
#   npm-dist-tag.sh VERSION [CURRENT_LATEST]
#
#   VERSION         the release version, MAJOR.MINOR.PATCH, for example 0.4.0
#   CURRENT_LATEST  the version the "latest" dist-tag names now, if any
#
# It prints "latest" when there is no current latest, or VERSION is not older
# than it, so "latest" never moves backward. Otherwise, for example when an
# older release is backfilled, it prints "release-MAJOR.MINOR", a tag that is
# not a semver range, as npm requires. Prereleases and any other form are
# refused: the release guard admits only vMAJOR.MINOR.PATCH tags.

set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
	echo "usage: npm-dist-tag.sh VERSION [CURRENT_LATEST]" >&2
	exit 2
fi
version=$1
current=${2:-}

release='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
printf '%s\n' "$version" | grep -Eq "$release" || {
	echo "npm-dist-tag: '$version' is not a MAJOR.MINOR.PATCH release version" >&2
	exit 1
}

if [ -z "$current" ]; then
	echo latest
	exit 0
fi
printf '%s\n' "$current" | grep -Eq "$release" || {
	echo "npm-dist-tag: the current latest, '$current', is not a MAJOR.MINOR.PATCH release version" >&2
	exit 1
}

# older A B: true when A is a lower version than B.
older() {
	echo "$1 $2" | awk '{
		split($1, a, "."); split($2, b, ".")
		for (i = 1; i <= 3; i++) {
			if (a[i] + 0 < b[i] + 0) exit 0
			if (a[i] + 0 > b[i] + 0) exit 1
		}
		exit 1
	}'
}

if older "$version" "$current"; then
	echo "release-$(echo "$version" | cut -d. -f1-2)"
else
	echo latest
fi
