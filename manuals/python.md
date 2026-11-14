---
name: python
version: "0.1"
status: provisional
description: "Baseline guidance for designing and building software in Python: module and package boundaries, dependencies and environments, entry points, types and data, exceptions, concurrency and cancellation, interfaces and seams, configuration, external boundaries, and testing."
---

# Python Manual

This is the Garage-maintained baseline guidance for designing and building software in Python. It is portable: it describes practice for any Python project, not the choices of a particular project. A project adopts it through its own profile and records its deltas there. See [Manuals](README.md) for what adoption means, and for this guidance's relationship to canonical requirements.

This baseline is deliberately narrow. It covers the language, standard library, and general engineering decisions most Python projects make early and most often. It sets no folder layout beyond what packaging needs, and gives no web-framework, data-science, or notebook guidance. Choose modules and packages from the capabilities a project actually has.

The guidance assumes Python 3. Where a practice depends on a later Python version, the minimum version is named. The standard library and the packaging standards, such as `venv`, `pip`, and `pyproject.toml`, are named directly. Other tools are described by category. An example of one is a common choice, not a default.

## Module and package boundaries

- Organize modules and packages by responsibility, not by technical kind. Avoid modules such as `utils`, `helpers`, or `common`, which collect unrelated code.
- Add a package when a capability needs a boundary of its own. Don't add one in anticipation of future capabilities. A module is often enough.
- Give names that are private to a module a leading underscore, and keep each module's public surface to what callers need. Where a module's public names should be explicit, list them in `__all__`.
- Importing a module shouldn't do work. Keep network calls, file writes, configuration loading, and logging setup out of module top level, and run them from an entry point.
- When the code is distributed as a package, prefer the `src/` layout, which keeps tests and tools running against the installed package rather than the working directory.

## Dependencies and environments

- Declare project metadata and dependencies in the `[project]` table of `pyproject.toml` (PEP 621). Set `requires-python` deliberately, to the oldest version the project actually supports, and don't add an upper bound without a reason.
- Prefer the standard library. Add a third-party package when it provides a capability the project needs and the standard library doesn't reasonably cover. Record why a significant dependency was chosen.
- Install into an isolated virtual environment (`venv`, 3.3), not into the system interpreter.
- Give a library's dependencies compatible version ranges. For an application, commit a lock file that pins the full dependency set, in the format the project's tool produces. `pylock.toml` (PEP 751) is the standard format.
- Keep development tools, such as test runners, linters, and type checkers, apart from runtime dependencies, for example in dependency groups (PEP 735) or optional extras.

## Entry points

- Keep the entry point thin. A `main(argv)` function parses arguments, runs the behavior, and returns an exit code. `__main__.py`, or a console script declared under `[project.scripts]`, calls it and passes the code to `sys.exit`.
- Keep argument parsing, help text, and output formatting separate from the behavior a command invokes. The behavior should be callable and testable without parsing arguments.
- Write output to streams that are passed in, rather than to `sys.stdout` and `sys.stderr` from behavior code, so tests can capture it.
- Report failure through a non-zero exit code and one clear message on standard error. Don't report the same failure twice, and don't let a traceback stand in for a message a user can act on.

## Types and data

- Annotate public functions, methods, and classes, and check the annotations with a static type checker as part of the project's checks. Mypy and Pyright are common choices.
- Keep `typing.Any` (3.5) out of public interfaces. Narrow data from outside to a precise type where it enters.
- Represent structured data with a class that names its fields, such as a dataclass (3.7), and mark it `frozen=True` where its values don't change after construction. Don't pass dictionaries of loosely related values through many layers.
- Mark a value that may be absent as `X | None` (3.10) or `Optional[X]` (3.5), and handle the absent case explicitly.
- Don't use a mutable value, such as a list or a dict, as a default argument. Default to `None`, and create the value inside the function.

## Errors and exceptions

- Raise and catch specific exception types. Don't use a bare `except:`. Catch `Exception` only at a boundary, such as the entry point or a unit of work, where the failure is reported.
- When translating an exception, chain the cause with `raise NewError(...) from err`, so the original is kept.
- Define a custom exception type only where a caller branches on it. Never branch on an exception's message.
- Release resources with `with` and context managers, rather than with cleanup code an exception can skip.
- Write messages for the person who will read them. Name the thing that failed and, where it helps, what to do next.
- Handle each failure once. Either handle it or let it propagate. Logging it and also re-raising it reports the same failure twice.
- In a library, log through `logging.getLogger(__name__)`, and leave handlers and levels to the application.

## Concurrency, async, and cancellation

- Choose a concurrency model deliberately: threads or processes (`concurrent.futures`, 3.2) for blocking work, or `asyncio` (3.4) for many concurrent waits. Don't combine them without a reason.

When code uses `asyncio`:

- Don't block the event loop. Run a blocking call in a thread with `asyncio.to_thread` (3.9).
- Start related tasks in an `asyncio.TaskGroup` (3.11), which waits for all of them and cancels the rest when one fails. Don't leave tasks running without an owner.
- Bound waits with `asyncio.timeout` (3.11).
- Let `asyncio.CancelledError` propagate. Clean up in `finally` or a context manager, and re-raise it if you catch it. From 3.8 it derives from `BaseException`, so `except Exception` doesn't catch it.
- An operation that can't wait doesn't need to be async. Make it async when it gains I/O.

## Interfaces and seams

- Define a `typing.Protocol` (3.8) where it is consumed, when the consumer needs substitution. Don't write an abstract base class in anticipation of a second implementation.
- Keep protocols small. A protocol with one or two methods is easier to satisfy, fake, and understand.
- Pass dependencies in as parameters or constructor arguments. Don't reach for module globals, and don't make tests patch them to substitute behavior.
- Prefer a callable parameter when only one operation needs to be substituted.

## Configuration

- Give persisted configuration and data formats a schema version from their first release, so their layout can change deliberately later.
- Record only values with an established purpose. Unused keys become implied commitments.
- Parse and validate configuration at startup into typed values, and report invalid values with their location and the expected form. Don't fail later at the point of use. The standard library reads TOML with `tomllib` (3.11).
- Never overwrite a user's existing configuration silently. Make replacement an explicit choice, and write it safely, as described under [External boundaries](#external-boundaries).
- Keep secrets out of committed files. Supply them from the environment or a secret store.

## External boundaries

- Isolate each external system, such as the filesystem, network services, subprocesses, and the clock, behind a boundary owned by the code that needs it. Keep its details out of core behavior. Pass the clock in, rather than reading the current time throughout.
- Use `pathlib` (3.4) for paths. To replace a file that must stay intact, write a temporary file in the same directory, then move it over the original with `os.replace` (3.3), which is atomic when it succeeds.
- Set a timeout on every network call. A client without one can wait indefinitely.
- Run subprocesses with an argument list, for example with `subprocess.run` (3.5), not with `shell=True` and a command string built from input. Check the exit status.
- Never unpickle data from an untrusted source, because `pickle` can run arbitrary code. Load YAML and similar formats from untrusted input only with a loader that builds plain data.
- Treat input from an external system as untrusted until validated, including file contents, paths, and responses.
- Isolate platform-specific behavior behind one boundary, checked with `sys.platform` or `os.name`, and express the behavior for each supported platform.

## Testing and validation

- Use one test framework per project. `unittest` ships with Python, pytest is a common alternative, and the choice is the project's.
- Test behavior that users and callers observe, such as outputs, files written, exit codes, and exception types, rather than internal implementation details.
- Test failure paths, not only the nominal path. Inject the failure through a seam the code already has, such as a parameter, a protocol, or a callable.
- Keep tests independent of the developer's environment and of one another. Give filesystem tests their own temporary directory, and don't depend on the current time, locale, time zone, or environment variables.
- Use parameterized tests where cases share structure, and name each case for the behavior it checks.
- Run a formatter, a linter, a type checker, and the tests on every change. Ruff, Black, mypy, and Pyright are examples, not defaults. Run the tests on each Python version the project supports.
- Run the same checks in continuous integration that contributors run locally.
