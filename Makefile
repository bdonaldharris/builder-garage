# Builder Garage CLI: build and validation commands.
#
# Every target wraps plain Go toolchain commands, which are also listed in
# README.md for environments without make (for example, Windows).

BINARY := garage
PKG    := ./cmd/garage
DIST   := dist

# Version reported by "garage version", derived from this checkout's Git state
# (docs/adr/0002-semantic-versioning.md):
#   - the tag at a clean tagged commit, for example v0.1.0;
#   - Git's descriptive form after it, for example v0.1.0-3-gabc1234;
#   - the abbreviated commit when no version tag is reachable, for example in
#     a shallow clone without tags;
#   - "-dirty" appended when the checkout has local changes, untracked files
#     included;
#   - "dev" without Git metadata, for example in an extracted source archive.
# install.sh derives it the same way. Neither writes to the checkout:
# "git describe --dirty" would rewrite the index, so modifications are detected
# with "git status" and GIT_OPTIONAL_LOCKS=0 instead.
# Override it for a release-style build:
#   make build VERSION=1.2.3
ifeq ($(origin VERSION),undefined)
VERSION := $(shell \
	if command -v git >/dev/null 2>&1 && \
	   top=$$(git -C "$(CURDIR)" rev-parse --show-toplevel 2>/dev/null) && \
	   [ "$$(cd "$$top" && pwd -P)" = "$$(cd "$(CURDIR)" && pwd -P)" ] && \
	   v=$$(git -C "$(CURDIR)" describe --tags --match 'v[0-9]*' --always 2>/dev/null); then \
		if [ -n "$$(GIT_OPTIONAL_LOCKS=0 git -C "$(CURDIR)" status --porcelain 2>/dev/null)" ]; then \
			v=$$v-dirty; \
		fi; \
		echo "$$v"; \
	else \
		echo dev; \
	fi)
endif

# CGO is disabled so the executables are self-contained: they need no Go
# installation at run time, and the Linux builds are statically linked.
GOBUILD := CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$(VERSION)"

PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: build version test fmt-check vet cross check clean

## build: compile ./garage for the current platform
build:
	$(GOBUILD) -o $(BINARY) $(PKG)

## version: print the version "make build" would report
version:
	@echo '$(VERSION)'

## test: run all tests
test:
	go test ./...

## fmt-check: fail if any Go file is not gofmt-formatted
fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not gofmt-formatted (run: gofmt -w .):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

## vet: run static analysis
vet:
	go vet ./...

## cross: cross-compile for every supported platform into dist/
cross:
	@set -e; \
	for platform in $(PLATFORMS); do \
		os="$${platform%/*}"; arch="$${platform#*/}"; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		out="$(DIST)/$(BINARY)_$${os}_$${arch}$${ext}"; \
		echo "building $$out"; \
		GOOS="$$os" GOARCH="$$arch" $(GOBUILD) -o "$$out" $(PKG); \
	done

## check: run every validation step
check: fmt-check vet test build cross

## clean: remove build outputs
clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(DIST)
