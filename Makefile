###
### Makefile Navigation
###
#
# 1. Variables & Definitions
# 2. Build: build
# 3. Validation: vet, test
# 4. Cleanup: clean
#
###
### Variables & Definitions
###

# Resolved through PATH so native Windows make finds Git Bash.
SHELL := bash

GO ?= go
PROJECT := wslc-bridge
DIST ?= dist
BUILDFLAGS ?= -trimpath
# An environment VERSION wins over this default; the release workflow sets it.
VERSION ?= 0.0.1-dev
BUILD_ARCH ?= amd64 arm64
COMMIT_NO ?= $(shell git rev-parse --short HEAD 2> /dev/null || true)
GIT_COMMIT ?= $(if $(shell git status --porcelain --untracked-files=no),$(COMMIT_NO)-dirty,$(COMMIT_NO))
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -s -w \
	-X $(PROJECT)/internal/buildinfo.Version=$(VERSION) \
	-X $(PROJECT)/internal/buildinfo.Commit=$(GIT_COMMIT) \
	-X $(PROJECT)/internal/buildinfo.Date=$(BUILD_DATE)

###
### Build
###

# The first target is the default, so a bare `make` builds everything.
.PHONY: build
build:
	@mkdir -p $(DIST)
	for arch in $(BUILD_ARCH); do \
		CGO_ENABLED=0 GOOS=windows GOARCH=$$arch $(GO) build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' \
			-o $(DIST)/$(PROJECT)-windows-$$arch.exe ./cmd/$(PROJECT) || exit 1; \
	done
	$(GO) run ./tools/buildmanifest -dist $(DIST) -version $(VERSION) -commit $(GIT_COMMIT) -date $(BUILD_DATE) -arch '$(BUILD_ARCH)'

###
### Validation
###

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test
test:
	$(GO) test ./...

###
### Cleanup
###

.PHONY: clean
clean:
	rm -rf $(DIST)
