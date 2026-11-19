# Security Policy

## Scope

Builder Garage contains a command-line application alongside engineering skills, standards, and workflow definitions that AI agents and people follow. It publishes releases of the command-line application on GitHub.

A security issue may affect the CLI, its generated project configuration, or content that could lead an agent or a person following it to expose credentials or other secrets, or to take unsafe or destructive actions. Reports about any other security-relevant weakness are also welcome.

## Supported versions

The latest published release is supported. Reports are assessed against it, and against the current state of the `staging` branch, where the next release is integrated. A fix to a released version ships as a hotfix to `main`, under [GOVERNANCE: Hotfixes and staging synchronization](GOVERNANCE.md#hotfixes-and-staging-synchronization), and is then synchronized into `staging`.

## Reporting a vulnerability

**Do not report security vulnerabilities in public issues, pull requests, or discussions.**

Report privately through one of these channels:

1. **GitHub private vulnerability reporting (preferred).** On the repository's **Security** tab, choose **Report a vulnerability**, or go to <https://github.com/bdonaldharris/builder-garage/security/advisories/new>.
2. **Email.** If that option is not available to you, write to <bdonaldharris@notablebit.com>.

Please include:

- the affected file, skill, or workflow;
- what could go wrong and under what conditions;
- steps or an example that demonstrates the problem, if you have one;
- any suggested correction.

## What to expect

A maintainer will acknowledge your report, investigate it, and keep you informed of the outcome. Builder Garage is a maintainer-led project without a dedicated security team, so no specific response time is guaranteed.

Please allow maintainers a reasonable opportunity to correct the problem before disclosing it publicly. Reporters are credited for their findings unless they prefer otherwise.
