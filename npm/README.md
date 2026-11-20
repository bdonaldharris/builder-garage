# @buildergarage/garage

The `garage` command-line interface of [Builder Garage](https://github.com/bdonaldharris/builder-garage), installed with npm.

```sh
npm install -g @buildergarage/garage
garage version
```

Or run it once, without installing it:

```sh
npx @buildergarage/garage version
```

**Always use the full name, `@buildergarage/garage`.** The unscoped npm package `garage` is unrelated to Builder Garage. `npx garage` runs that package instead, and it also installs a `garage` command, so npm refuses to install both.

## What it installs

This package is a small launcher. npm installs one platform package beside it, `@buildergarage/garage-<os>-<cpu>`, which holds the `garage` executable from the matching GitHub release archive, byte for byte. The launcher runs that executable. It needs Node.js 20 or later.

| Platform | Platform package |
| --- | --- |
| macOS, Apple silicon | `@buildergarage/garage-darwin-arm64` |
| macOS, Intel | `@buildergarage/garage-darwin-x64` |
| Linux, arm64 | `@buildergarage/garage-linux-arm64` |
| Linux, x86-64 | `@buildergarage/garage-linux-x64` |

**Windows is not offered through npm yet.** On Windows, the launcher says so. Download and verify a release archive instead. See [Verifying a release](https://github.com/bdonaldharris/builder-garage/blob/main/docs/release-verification.md#npm) for platform trust on each platform.

Upgrade with `npm install -g @buildergarage/garage@latest`, and remove it with `npm uninstall -g @buildergarage/garage`.

## Verifying it

Every package is published from Builder Garage's own GitHub Actions workflow, with npm provenance. Check it with `npm audit signatures`, in a project that depends on it. The executable's bytes are those of the release archive, which is covered by the release's signed checksum manifest. [Verifying a release](https://github.com/bdonaldharris/builder-garage/blob/main/docs/release-verification.md#npm) explains both checks.

## License

MIT
