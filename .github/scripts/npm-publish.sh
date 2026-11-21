#!/bin/sh
# npm-publish.sh: publish the npm packages npm-packages.sh generated (#158).
#
#   npm-publish.sh PACKAGES DIST_TAG [--dry-run]
#
#   PACKAGES  the directory npm-packages.sh wrote
#   DIST_TAG  the dist-tag to publish under, from npm-dist-tag.sh
#   --dry-run run "npm publish --dry-run", without provenance, so nothing is
#             published
#
# It publishes every platform package, then the launcher, so a launcher
# version is never public before the platform packages it names. Each is
# published public, with provenance, under DIST_TAG.
#
# It can be run again after a partial failure. A package version already on
# the registry is not published again: it is downloaded and compared with the
# one generated here (the executable for a platform package; the launcher
# script and its optionalDependencies for the launcher), and a difference
# fails the run. Any other registry error fails the run too.
#
# It needs npm, and node to read package.json. Authentication is npm's: OIDC
# trusted publishing in the npm workflow, or a token for a first publish.

set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	echo "usage: npm-publish.sh PACKAGES DIST_TAG [--dry-run]" >&2
	exit 2
fi
packages=$1
tag=$2
dry=
if [ "$#" -eq 3 ]; then
	[ "$3" = --dry-run ] || {
		echo "usage: npm-publish.sh PACKAGES DIST_TAG [--dry-run]" >&2
		exit 2
	}
	dry=--dry-run
fi

refuse() {
	echo "npm-publish: $1" >&2
	exit 1
}

[ -d "$packages/garage" ] || refuse "$packages/garage does not exist; generate the packages with npm-packages.sh"
[ -n "$tag" ] || refuse "no dist-tag was given"

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
else
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-npm-publish.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

field() { node -p "require(process.argv[1])[process.argv[2]]" "$(CDPATH='' cd -- "$1" && pwd)/package.json" "$2"; }
deps() { node -p "JSON.stringify(require(process.argv[1]).optionalDependencies || {})" "$1/package.json"; }

# published NAME VERSION: true when NAME@VERSION is on the registry. A
# missing package or version is false; any other error stops the run.
published() {
	if npm view "$1@$2" version >"$work/view.out" 2>"$work/view.err"; then
		[ "$(cat "$work/view.out")" = "$2" ]
	elif grep -q E404 "$work/view.err"; then
		return 1
	else
		cat "$work/view.err" >&2
		refuse "cannot tell whether $1@$2 is published"
	fi
}

# same DIR NAME VERSION: the published NAME@VERSION matches DIR.
same() {
	rm -rf "$work/pub"
	mkdir -p "$work/pub"
	(cd "$work/pub" && npm pack "$2@$3" --silent >/dev/null) || refuse "cannot download $2@$3 to compare it"
	tar -xzf "$work"/pub/*.tgz -C "$work/pub"
	if [ -f "$1/bin/garage" ]; then
		[ "$(sha256 "$work/pub/package/bin/garage")" = "$(sha256 "$1/bin/garage")" ]
	else
		[ "$(sha256 "$work/pub/package/bin/garage.js")" = "$(sha256 "$1/bin/garage.js")" ] &&
			[ "$(deps "$work/pub/package")" = "$(deps "$1")" ]
	fi
}

publish() {
	dir=$1
	name=$(field "$dir" name)
	version=$(field "$dir" version)
	if published "$name" "$version"; then
		if same "$dir" "$name" "$version"; then
			echo "npm-publish: $name@$version is already published, and matches; not publishing it again"
			return 0
		fi
		refuse "$name@$version is already published, and differs from the package generated here"
	fi
	if [ -n "$dry" ]; then
		npm publish "$dir" --access public --tag "$tag" --dry-run
	else
		npm publish "$dir" --access public --tag "$tag" --provenance
	fi
	echo "npm-publish: published $name@$version under $tag${dry:+ (dry run)}"
}

for dir in "$packages"/garage-*; do
	[ -d "$dir" ] || refuse "no platform package was generated in $packages"
	publish "$dir"
done
# npm can accept a publish before every registry replica serves the version.
# Do not make the launcher visible until all packages it names are served.
launcher_deps=$(node -e 'console.log(Object.keys(require(process.argv[1]).optionalDependencies || {}).join("\\n"))' "$packages/garage/package.json")
[ -n "$launcher_deps" ] || refuse "$packages/garage has no platform optionalDependencies"
# Package names contain no whitespace. Keep the dependency source of truth in
# the launcher package rather than duplicate the platform list here.
set -- $launcher_deps
if [ -z "$dry" ]; then
	sh "$scripts/npm-wait-for-packages.sh" "$(field "$packages/garage" version)" "$@"
else
	echo "npm-publish: dry run; not waiting for registry replication"
fi
publish "$packages/garage"
