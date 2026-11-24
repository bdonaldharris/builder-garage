#!/bin/sh
# Adopter release publishing only. It uploads release files to this project's
# generic package registry with the CI job token, and writes a manifest of what
# it published. It creates no tag, release or release link, runs no agent, and
# never overwrites a published file.
set -eu
umask 077

fail() {
  printf 'garage release publish: %s\n' "$1" >&2
  exit 1
}

# Accept one generic-package path segment: a package name, version or file name.
segment() {
  case $1 in
    '' | . | .. | -*) return 1 ;;
    *[!A-Za-z0-9._+~-]*) return 1 ;;
  esac
  return 0
}

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

[ -n "${CI_COMMIT_TAG:-}" ] || fail 'a tag pipeline is required'
[ "${CI_COMMIT_REF_PROTECTED:-false}" = true ] || fail 'refusing to publish from an unprotected tag'
[ -n "${CI_API_V4_URL:-}" ] && [ -n "${CI_PROJECT_ID:-}" ] && [ -n "${CI_JOB_TOKEN:-}" ] || fail 'GitLab CI job variables are missing'
case $CI_API_V4_URL in https://*) ;; *) fail 'the GitLab API URL is not HTTPS' ;; esac
command -v curl >/dev/null 2>&1 || fail 'curl is missing; supply it in GARAGE_RELEASE_IMAGE'

dir=${GARAGE_RELEASE_DIR:-dist}
package=${GARAGE_RELEASE_PACKAGE:-release}
version=$CI_COMMIT_TAG
segment "$package" || fail "the package name is not a valid generic package segment: $package"
segment "$version" || fail "the tag is not a valid generic package version: $version"
[ -d "$dir" ] || fail "the release directory does not exist: $dir"

mkdir -p garage-release-evidence
manifest=garage-release-evidence/manifest.txt
# The manifest is a job-environment assertion. `garage forge release inspect`
# reads the registry's own digests; this file never replaces that check.
{
  printf '%s\n' 'garage_release_manifest=1'
  printf '%s\n' 'source=ci-job-environment-asserted'
  printf '%s\n' 'authority=none; not a release record, delegation or human acceptance'
  printf 'tag=%s\n' "$version"
  printf 'commit=%s\n' "${CI_COMMIT_SHA:-unavailable}"
  printf 'pipeline=%s\n' "${CI_PIPELINE_URL:-unavailable}"
  printf 'package=%s\n' "$package"
} > "$manifest"

count=0
for path in "$dir"/*; do
  [ -e "$path" ] || continue
  name=${path##*/}
  [ -f "$path" ] && [ ! -L "$path" ] || fail "not a regular file: $name"
  segment "$name" || fail "the file name is not a valid generic package file name: $name"
  url="$CI_API_V4_URL/projects/$CI_PROJECT_ID/packages/generic/$package/$version/$name"
  code=$(curl --silent --output /dev/null --write-out '%{http_code}' --header "JOB-TOKEN: $CI_JOB_TOKEN" "$url") || code=000
  case $code in
    404) ;;
    200) fail "already published, and published files are never replaced: $name" ;;
    *) fail "cannot establish whether $name is already published (HTTP $code)" ;;
  esac
  curl --fail --silent --show-error --output /dev/null --header "JOB-TOKEN: $CI_JOB_TOKEN" --upload-file "$path" "$url" ||
    fail "upload failed or is uncertain for $name; inspect the package registry before retrying"
  printf 'file=%s sha256=%s url=%s\n' "$name" "$(digest "$path")" "$url" >> "$manifest"
  count=$((count + 1))
done
[ "$count" -gt 0 ] || fail "no release files in $dir"
cat "$manifest"
