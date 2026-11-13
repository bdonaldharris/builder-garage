---
name: go
version: "0.1"
status: provisional
description: "Baseline guidance for designing and building software in Go: package boundaries, dependency direction, errors, contexts, interfaces, configuration, external boundaries, and testing."
---

# Go Manual

This is the Garage-maintained baseline guidance for designing and building software in Go. It is portable: it describes practice for any Go project, not the choices of a particular project. A project adopts it through its own profile and records its deltas there. See [Manuals](README.md) for what adoption means, and for this guidance's relationship to canonical requirements.

This baseline is deliberately narrow. It covers the decisions most Go projects make early and most often. It sets no package layout. Choose packages from the capabilities a project actually has.

## Package boundaries

- Organize packages by responsibility, not by technical kind. Avoid packages such as `models`, `utils`, `helpers`, or `common`, which collect unrelated code.
- Add a package when an established capability needs a boundary of its own. Don't add one in anticipation of future capabilities. One well-named package is better than several speculative ones.
- Use `internal/` for code that other modules should not import. Promote code out of `internal/` only when an external consumer actually needs it.
- Keep a package's exported surface small. Export what callers need, and leave the rest unexported.
- Name packages with short, lowercase nouns that describe what they provide. Avoid repeating the package name in its identifiers (`config.Load`, not `config.LoadConfig`).

## Dependency direction

- Dependencies point from entry points and adapters toward the core behavior they use, not the other way. Core behavior shouldn't import the command framework, transport, or storage that calls it.
- Import cycles are a design signal. Resolve one by moving the shared responsibility or inverting the dependency, not by merging packages without regard to responsibility.
- Prefer the standard library. Add a third-party module when it provides a capability the project needs and the standard library doesn't reasonably cover. Record why a significant dependency was chosen.
- Manage dependencies with Go modules. Keep `go.mod` and `go.sum` committed and tidy, and keep the declared Go version current deliberately rather than incidentally.

## Command-line separation

- Keep `main` packages thin. A `main` function builds the command, runs it, and turns failure into an exit status. Behavior lives elsewhere.
- Keep command wiring, such as flags, arguments, help text, and output formatting, separate from the behavior a command invokes. The behavior should be callable and testable without parsing arguments.
- Write command output to the writers the command framework provides rather than directly to `os.Stdout` and `os.Stderr`, so tests can capture it.
- Report failure through a non-zero exit status and a clear message on standard error. Don't print the same error twice.
- Expose only commands that are deliberately implemented. Disable generated commands the project hasn't chosen to support.

## Errors

- Return errors. Reserve `panic` for programming errors that should never happen, not for conditions a caller or user can cause.
- Add the operation that failed when passing an error up: `fmt.Errorf("cannot read the configuration: %w", err)`. Wrap with `%w` when callers may need the cause, and avoid adding context that only repeats what the wrapped error already says.
- Define sentinel or typed errors only where a caller needs to branch on the failure. Check them with `errors.Is` and `errors.As`, never by comparing message strings.
- Write error messages for the person who will read them. Name the thing that failed and, where it helps, what to do next.
- Handle each error once. Either handle it or return it. Logging and also returning it reports the same failure twice.

## Contexts

- Pass a `context.Context` as the first parameter of operations that can block, wait, or call an external system, such as network, subprocess, or long-running filesystem work.
- Don't store a context in a struct, and don't pass `nil`. Use `context.Background()` at the top of a program and in tests, and derive from it.
- Honor cancellation. Long-running work should stop promptly when its context is done and return the context's error.
- Use context values only for request-scoped data that crosses API boundaries. Don't use them to pass optional parameters.
- An operation that can't block doesn't need a context. Add one when an operation gains a blocking or external dependency.

## Interfaces

- Define an interface where it is consumed, when the consumer needs substitution. Don't define one beside its only implementation in anticipation of a second.
- Keep interfaces small. An interface with one or two methods is easier to satisfy, fake, and understand.
- Accept interfaces and return concrete types, unless the concrete type is deliberately hidden.
- Prefer a function value when only one operation needs to be substituted. A function parameter is often a simpler seam than an interface.

## Configuration

- Give configuration a schema version from its first release, so its layout can change deliberately later.
- Record only values with an established purpose. Unused keys become implied commitments.
- Validate configuration when it is loaded, and report invalid values with their location and the expected form. Don't fail later at the point of use.
- Never overwrite a user's existing configuration silently. Make replacement an explicit choice, and write it safely, for example to a temporary file that then replaces the original.
- Keep secrets out of committed configuration files.

## External boundaries

- Isolate each external system, such as the filesystem, network services, subprocesses, and the clock, behind a boundary owned by the code that needs it. Keep its details out of core behavior.
- Treat input from an external system as untrusted until validated, including file contents, paths, and responses.
- Be deliberate about filesystem effects. Check for symbolic links, existing files, and partial writes where an operation could damage user data, and prefer atomic replacement for files that must stay intact.
- Isolate platform-specific behavior in files selected by build constraints (`//go:build windows`), with the same behavior expressed for each supported platform.
- Set timeouts on calls to external services. Don't let a failure in an external system hang the program.

## Testing and validation

- Use the standard `testing` package. Add a test library only for a capability the standard library lacks.
- Test behavior that users and callers observe, such as outputs, files written, exit status, and error identity, rather than internal implementation details.
- Use `t.TempDir()` for filesystem tests, and keep tests independent of the developer's environment and of one another.
- Use table-driven tests where cases share structure, and name each case for the behavior it checks.
- Test failure paths, not only the nominal path. Inject the failure through the seam the code already has.
- Run `gofmt` and `go vet` on every change, together with the tests. Build for each supported platform when a project ships for several.
- Run the same checks in continuous integration that contributors run locally.
