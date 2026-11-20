#!/bin/sh
# check-npm-install.sh: install garage with npm, check it, and remove it
# (#158).
#
#   check-npm-install.sh --local PACKAGES BINARY_VERSION REVISION [OLDER_PACKAGES]
#   check-npm-install.sh --registry VERSION BINARY_VERSION REVISION
#
#   --local     install the packages npm-packages.sh wrote to PACKAGES, from
#               locally packed tarballs, as pull-request CI does before
#               anything is published. With OLDER_PACKAGES, it first installs
#               those, then upgrades to PACKAGES.
#   --registry  install @buildergarage/garage@VERSION from the npm registry,
#               as a user would, after waiting a bounded time for it to
#               appear, and check its signatures with "npm audit signatures"
#
# Everything happens under a temporary npm prefix and cache, which is
# removed afterwards, so the machine's own npm installation is untouched.
# It checks that:
#   - npm install -g installs the launcher and exactly one platform package,
#     the one for this machine;
#   - the installed garage, run through the launcher, passes
#     check-release-archive.sh: it reports BINARY_VERSION, and installs
#     skills whose revision is REVISION;
#   - npx runs garage version;
#   - npm uninstall -g removes garage.
#
# It needs node and npm, and runs on macOS and Linux.

set -eu

usage() {
	echo "usage: check-npm-install.sh --local PACKAGES BINARY_VERSION REVISION [OLDER_PACKAGES]" >&2
	echo "       check-npm-install.sh --registry VERSION BINARY_VERSION REVISION" >&2
	exit 2
}
[ "$#" -ge 1 ] || usage
mode=$1
shift
case $mode in
--local) [ "$#" -eq 3 ] || [ "$#" -eq 4 ] || usage ;;
--registry) [ "$#" -eq 3 ] || usage ;;
*) usage ;;
esac

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
name=@buildergarage/garage

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-npm-check.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

npm_config_prefix=$work/prefix
npm_config_cache=$work/cache
npm_config_update_notifier=false
npm_config_fund=false
npm_config_audit=false
export npm_config_prefix npm_config_cache npm_config_update_notifier npm_config_fund npm_config_audit
mkdir -p "$work/prefix" "$work/run"
garage=$work/prefix/bin/garage
host=$(node -p 'process.platform + "-" + process.arch')

# pack_local DIR OUT: pack every package in DIR into OUT, with the launcher's
# optionalDependencies pointed at the packed platform tarballs, and print the
# launcher tarball's path.
pack_local() {
	mkdir -p "$2"
	for p in "$1"/garage-*; do
		(cd "$p" && npm pack --pack-destination "$2" --silent >/dev/null)
	done
	cp -R "$1/garage" "$2/launcher"
	node -e '
		const fs = require("fs"), path = require("path");
		const [file, dir] = process.argv.slice(1);
		const pkg = JSON.parse(fs.readFileSync(file));
		for (const dep of Object.keys(pkg.optionalDependencies)) {
			const t = `${dep.replace(/^@/, "").replace("/", "-")}-${pkg.optionalDependencies[dep]}.tgz`;
			if (!fs.existsSync(path.join(dir, t))) throw new Error(`missing ${t}`);
			pkg.optionalDependencies[dep] = "file:" + path.join(dir, t);
		}
		fs.writeFileSync(file, JSON.stringify(pkg, null, 2));
	' "$2/launcher/package.json" "$2"
	(cd "$2/launcher" && npm pack --pack-destination "$2" --silent)
}

# installed_platforms: the platform packages installed beside the launcher.
installed_platforms() {
	ls "$work/prefix/lib/node_modules/$name/node_modules/@buildergarage" 2>/dev/null |
		sed 's/^garage-//' | tr '\n' ' ' | sed 's/ $//'
}

install() { # install SPEC LABEL
	if (cd "$work/run" && npm install -g "$1" >"$work/install.out" 2>&1); then pass "npm install -g $2"; else
		cat "$work/install.out"
		fail "npm install -g $2"
	fi
}

case $mode in
--local)
	packages=$1
	bver=$2
	rev=$3
	older=${4:-}
	if [ -n "$older" ]; then
		old=$(pack_local "$older" "$work/older")
		install "$work/older/$old" "(older packages)"
		oldver=$(node -p 'require(process.argv[1]).version' "$older/garage/package.json")
		# A release reports its tag (vX.Y.Z); a snapshot reports its version.
		out=$("$garage" version 2>&1 || true)
		case $out in
		"garage version $oldver" | "garage version v$oldver") pass "the older garage reports $oldver" ;;
		*) fail "the older garage reports $oldver (got: $out)" ;;
		esac
	fi
	spec=$work/new/$(pack_local "$packages" "$work/new")
	label="(packages from $packages)"
	;;
--registry)
	version=$1
	bver=$2
	rev=$3
	spec=$name@$version
	label=$spec
	# The registry may take a while to serve a new version.
	i=0
	until [ "$(npm view "$spec" version 2>/dev/null)" = "$version" ]; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || break
		sleep 15
	done
	;;
esac

install "$spec" "$label"
got=$(installed_platforms)
if [ "$got" = "$host" ]; then pass "only the $host platform package is installed"; else fail "only the $host platform package is installed (got: ${got:-none})"; fi

if [ -x "$garage" ]; then
	# check-release-archive.sh runs garage from an archive; this one runs the
	# npm-installed launcher, so the check goes through npm's bin and Node.
	mkdir -p "$work/shim"
	printf '#!/bin/sh\nexec "%s" "$@"\n' "$garage" >"$work/shim/garage"
	chmod 0755 "$work/shim/garage"
	tar -czf "$work/installed.tar.gz" -C "$work/shim" garage
	if sh "$scripts/check-release-archive.sh" "$work/installed.tar.gz" "$bver" "$rev"; then
		pass "the npm-installed garage passes check-release-archive.sh"
	else
		fail "the npm-installed garage passes check-release-archive.sh"
	fi
else
	fail "npm put garage on its bin ($garage)"
fi

out=$(cd "$work/run" && npm_config_cache=$work/npx-cache npx --yes --package="$spec" garage version 2>&1 || true)
if [ "$out" = "garage version $bver" ]; then pass "npx runs garage version"; else fail "npx runs garage version (got: $out)"; fi

if [ "$mode" = --registry ]; then
	proj=$work/audit
	mkdir -p "$proj"
	if (cd "$proj" && npm init -y >/dev/null && npm install "$spec" >"$work/audit-install.out" 2>&1 &&
		npm audit signatures >"$work/audit.out" 2>&1); then
		pass "npm audit signatures verifies $spec"
		cat "$work/audit.out"
	else
		cat "$work/audit-install.out" "$work/audit.out" 2>/dev/null || true
		fail "npm audit signatures verifies $spec"
	fi
fi

if (cd "$work/run" && npm uninstall -g "$name" >"$work/uninstall.out" 2>&1); then pass "npm uninstall -g $name"; else
	cat "$work/uninstall.out"
	fail "npm uninstall -g $name"
fi
if [ ! -e "$garage" ]; then pass "uninstalling removes $garage"; else fail "uninstalling removes $garage"; fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all npm checks passed for $label"
