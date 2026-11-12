# Inspection

Inspection covers independent review, focused tests, acceptance verification, regression checks, and quality gates.

The governing application review contract asks:

1. Were the acceptance criteria met?
2. Did the implementation introduce a new bug or regression?
3. Does the implemented behavior work?

`Yes -> No -> Yes` is the approval disposition under the current contract. Testing requirements remain specific to the application and change. A review is not complete until its required durable Issue/PR record is written and verified.

See [Independent Implementation Review](../skills/independent-implementation-review/SKILL.md) and the [root review contract](../README.md#review-contract).

## Boundary

Inspection owns the judgment of *whether work is correct*. It does not implement or correct the work it reviews; that stays with the implementing role defined by the [Crew](../crew/).

Independent engineering review is distinct from a maintainer's approval of a pull request. The first determines whether a change is correct for its issue; the second determines whether the project accepts it. See [GOVERNANCE.md](../GOVERNANCE.md#review-requirements).

## Status

This directory contains documentation only. No automated checks or test tooling have been implemented. See the [architectural direction](../docs/architecture.md).
