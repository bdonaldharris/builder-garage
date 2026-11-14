---
name: dotnet
version: "0.1"
status: provisional
description: "Baseline guidance for designing and building software in C# on .NET: project and namespace boundaries, dependencies, entry points, types and nullability, exceptions, async and cancellation, interfaces and composition, configuration, external boundaries, and testing."
---

# C# / .NET Manual

This is the Garage-maintained baseline guidance for designing and building software in C# on .NET. It is portable: it describes practice for any C# project, not the choices of a particular project. A project adopts it through its own profile and records its deltas there. See [Manuals](README.md) for what adoption means, and for this guidance's relationship to canonical requirements.

This baseline is deliberately narrow. It covers the language, runtime, and general engineering decisions most C# projects make early and most often. It sets no solution or folder layout, and gives no web-framework or application-host guidance. Choose projects and namespaces from the capabilities a project actually has.

Where a practice depends on a language, runtime, or SDK version, the minimum version is named. Tools that ship with .NET, such as the `dotnet` CLI, MSBuild, NuGet, and the built-in analyzers, are named directly. Other tools are described by category. An example of one is a common choice, not a default.

## Project and namespace boundaries

- Organize projects and namespaces by responsibility, not by technical kind. Avoid namespaces such as `Models`, `Helpers`, `Utilities`, or `Common`, which collect unrelated code.
- Add a project when a capability needs a boundary of its own, such as separate deployment, separate dependencies, or a dependency rule the compiler should enforce. Don't add one in anticipation of future capabilities. A namespace is often enough.
- Make types `internal` by default. Keep each project's `public` surface to what other projects actually need.
- Use `InternalsVisibleTo` sparingly. Granting a test project access is common. Granting it to another production project usually means the boundary is in the wrong place.
- Name namespaces and types for what they provide, and avoid repeating the namespace in type names.

## Dependency direction and packages

- References point from entry points and adapters toward the core behavior they use, not the other way. Core projects shouldn't reference the host, transport, or storage that calls them.
- Project references can't form a cycle. Treat pressure toward one as a design signal: move the shared responsibility, or invert the dependency.
- Prefer the base class library. Add a NuGet package when it provides a capability the project needs and the base class library doesn't reasonably cover. Record why a significant dependency was chosen.
- Manage package versions deliberately. In a solution with several projects, central package management keeps one version of each package in `Directory.Packages.props` (NuGet 6.2, .NET SDK 6.0.300). A lock file, `packages.lock.json` (NuGet 4.9), restored with `dotnet restore --locked-mode` in continuous integration, makes restores repeatable.
- Pin the SDK with `global.json`, and choose its `rollForward` policy (.NET Core 3.0 SDK) rather than accepting whichever SDK is installed.
- Choose each project's target framework deliberately, and move it forward as a decision, not as a side effect of upgrading tools.

## Entry points

- Keep `Program.cs` thin. It builds what the program needs, runs it, and turns failure into an exit code. Behavior lives elsewhere.
- Keep argument parsing, help text, and output formatting separate from the behavior a command invokes. The behavior should be callable and testable without parsing arguments.
- Write output to a `TextWriter` that is passed in, rather than to `Console.Out` and `Console.Error` from behavior code, so tests can capture it.
- Report failure through a non-zero exit code and one clear message on standard error. Don't report the same failure twice, and don't let an unhandled exception's stack trace stand in for a message a user can act on.

## Types and nullability

- Enable nullable reference types (`<Nullable>enable</Nullable>`, C# 8). New project templates enable them from .NET 6. Treat nullable warnings as defects, for example by making them errors in the build.
- Avoid the null-forgiving operator (`!`). When the compiler can't see that a value is non-null, restructure the code or validate the value where it enters.
- Validate arguments at public entry points, and let the type system carry the result inward.
- Prefer immutable data where state doesn't need to change: records and `init` accessors (C# 9), and read-only collection types in public signatures.
- Give a validated value its own type when it travels through several layers. A dedicated type says more than a raw string or integer.

## Errors and exceptions

- Use exceptions for conditions a caller can't reasonably prevent, such as a missing file, a failed network call, or invalid input from outside. Don't use them for ordinary control flow. Where failure is an expected outcome, return it, for example from a `Try…` method.
- Catch specific exception types. Catch broadly only at a boundary, such as the entry point or a unit of work, where the failure is reported.
- Rethrow with `throw;`, not `throw ex;`, so the original stack trace is kept. When wrapping, pass the original as the inner exception, and name the operation that failed.
- Define a custom exception type only where a caller branches on it. Never branch on an exception's message.
- Write messages for the person who will read them. Name the thing that failed and, where it helps, what to do next.
- Handle each failure once. Either handle it or let it propagate. Logging it and also rethrowing it reports the same failure twice.

## Async and cancellation

When code performs I/O or other work that waits:

- Make it async all the way. Don't block on a task with `.Result`, `.Wait()`, or `GetAwaiter().GetResult()`, which can deadlock and holds a thread while waiting.
- Return `Task` or `ValueTask` (.NET Core 2.1). Use `async void` only for event handlers, which have no alternative. A caller can't observe an exception from an `async void` method.
- Accept a `CancellationToken`, usually as the last parameter, on operations that can block, wait, or call an external system. Pass it on to the calls they make.
- Honor cancellation promptly. Long-running work should stop when cancellation is requested and let `OperationCanceledException` propagate, rather than reporting it as a failure.
- An operation that can't wait doesn't need to be async or accept a token. Add them when it gains an I/O or external dependency.

## Interfaces and composition

- Define an interface where it is consumed, when the consumer needs substitution. Don't add an interface beside every class in anticipation of a second implementation.
- Keep interfaces small. An interface with one or two members is easier to implement, fake, and understand.
- Pass dependencies in through constructors, so a type's dependencies are visible in its constructor. Don't fetch them from a global or a service locator.
- Wire the object graph in one place, the composition root, near the entry point. When a project uses a dependency-injection container, keep its registrations there, and keep core types unaware of the container.
- Prefer a delegate, such as `Func<…>`, when only one operation needs to be substituted.

## Configuration

- Give persisted configuration and data formats a schema version from their first release, so their layout can change deliberately later.
- Record only values with an established purpose. Unused keys become implied commitments.
- Validate configuration at startup, and report invalid values with their location and the expected form. Don't fail later at the point of use.
- Never overwrite a user's existing configuration silently. Make replacement an explicit choice, and write it safely, as described under [External boundaries](#external-boundaries).
- Keep secrets out of committed files, including settings files checked in beside the code. Supply them from the environment or a secret store.

## External boundaries

- Isolate each external system, such as the filesystem, network services, processes, and the clock, behind a boundary owned by the code that needs it. Keep its details out of core behavior.
- Read the time through `TimeProvider` (.NET 8, or the `Microsoft.Bcl.TimeProvider` package for earlier targets) rather than `DateTime.Now`, so time-dependent behavior can be tested.
- Set timeouts on calls to external services. Don't let a failure in an external system hang the program.
- Reuse long-lived `HttpClient` instances rather than creating one per call, and set `PooledConnectionLifetime` on `SocketsHttpHandler` (.NET Core 2.1) so they pick up DNS changes. When a project uses a dependency-injection container, `IHttpClientFactory`, from the `Microsoft.Extensions.Http` package (.NET Core 2.1), is an alternative.
- Dispose deterministically: `using` for `IDisposable`, and `await using` for `IAsyncDisposable` (C# 8, .NET Core 3.0).
- Be deliberate about filesystem effects. Check for symbolic links, existing files, and partial writes where an operation could damage user data. To replace a file that must stay intact, write a temporary file in the same directory, then move it over the original with `File.Move(source, destination, overwrite: true)` (.NET Core 3.0) or `File.Replace` (.NET Core 2.0).
- Treat input from an external system as untrusted until validated, including file contents, paths, and responses.
- Isolate platform-specific behavior behind one boundary, guarded with checks such as `OperatingSystem.IsWindows()` (.NET 5), and express the behavior for each supported platform.

## Testing and validation

- Use one test framework per project. xUnit, NUnit, and MSTest are common choices, and the choice is the project's. Add an assertion or test-double library only for a capability the framework lacks.
- Test behavior that users and callers observe, such as outputs, files written, exit codes, and exception types, rather than internal implementation details.
- Test failure paths, not only the nominal path. Inject the failure through a seam the code already has, such as an interface, a delegate, or a `TimeProvider`.
- Keep tests independent of the developer's environment and of one another. Give filesystem tests their own temporary directory, and don't depend on the current time, culture, or time zone.
- Use parameterized tests where cases share structure, and name each case for the behavior it checks.
- Run `dotnet build` with the built-in analyzers (on by default from .NET 5) and warnings treated as errors, `dotnet format --verify-no-changes` (.NET 6 SDK), and `dotnet test` on every change. Build for each supported platform when a project ships for several.
- Run the same checks in continuous integration that contributors run locally.
