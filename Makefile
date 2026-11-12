# Builder Garage CLI: build and validation commands.
#
# Every target wraps plain Go toolchain commands, which are also listed in
# README.md for environments without make (for example, Windows).

BINARY := garage
PKG    := ./cmd/garage
DIST   := dist

# Version reported by "garage version". Override for a release-style build:
#   make build VERSION=1.2.3
VERSION ?= dev

# CGO is disabled so the executables are self-contained: they need no Go
# installation at run time, and the Linux builds are statically linked.
GOBUILD := CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$(VERSION)"

PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: build test fmt-check vet cross check clean

## build: compile ./garage for the current platform
build:
	$(GOBUILD) -o $(BINARY) $(PKG)

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
