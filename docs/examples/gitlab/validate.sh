#!/bin/sh
# Adopter validation only: no credentials, network, writes to a forge, or agents.
set -eu
umask 077
mkdir -p garage-ci-evidence
# These are job-environment assertions, not provider-checked workflow records.
# A provider adapter must independently bind pipeline/job facts to an MR/head.
printf '%s\n' \
  'garage_ci_evidence=1' \
  'source=ci-job-environment-asserted' \
  'authority=none; not an Agent review, human acceptance, or merge permission' \
  "project=${CI_PROJECT_URL:-unavailable}" \
  "commit=${CI_COMMIT_SHA:-unavailable}" \
  "pipeline=${CI_PIPELINE_URL:-unavailable}" \
  "job=${CI_JOB_URL:-unavailable}" \
  "merge_request_iid=${CI_MERGE_REQUEST_IID:-not-an-mr-pipeline}" \
  > garage-ci-evidence/context.txt
if ! command -v garage >/dev/null 2>&1; then
  printf '%s\n' 'Garage is missing; supply the reviewed binary in GARAGE_CI_IMAGE.' >&2
  printf '%s\n' 'unavailable: garage executable missing' > garage-ci-evidence/config-validation.txt
  exit 1
fi
garage version > garage-ci-evidence/garage-version.txt
if garage config validate > garage-ci-evidence/config-validation.txt 2>&1; then
  cat garage-ci-evidence/config-validation.txt
else
  cat garage-ci-evidence/config-validation.txt >&2
  exit 1
fi
