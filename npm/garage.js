#!/usr/bin/env node
// The garage launcher for npm (@buildergarage/garage, #158).
//
// npm installs exactly one platform package beside this launcher, selected
// by its "os" and "cpu" fields, and that package holds the release's garage
// executable, byte for byte. This script finds it and runs it with the same
// arguments, standard streams, and environment, then exits as it exited.
// It adds no behavior of its own.
"use strict";

const { spawnSync } = require("node:child_process");

const docs =
  "https://github.com/bdonaldharris/builder-garage/blob/main/docs/release-verification.md";
const platform = `${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "garage.exe" : "garage";
const pkg = `@buildergarage/garage-${platform}`;

let binary;
try {
  binary = require.resolve(`${pkg}/bin/${exe}`);
} catch {
  console.error(
    `garage: npm does not provide garage for ${platform}, or ${pkg} was not installed.\n` +
      `Download and verify a release archive instead, as described in:\n  ${docs}`,
  );
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`garage: cannot run ${binary}: ${result.error.message}`);
  process.exit(1);
}
if (result.signal) {
  process.kill(process.pid, result.signal);
}
process.exit(result.status ?? 1);
