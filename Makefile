# Copyright (C) 2026 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

###
### Makefile Navigation
###
#
# 1. Variables & Definitions
# 2. Build: build
# 3. Validation: fmt, check-fmt, vet, test, test-coverage
# 4. Cleanup: clean
# 5. Help: help
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
	-X $(PROJECT)/pkg/buildinfo.Version=$(VERSION) \
	-X $(PROJECT)/pkg/buildinfo.Commit=$(GIT_COMMIT) \
	-X $(PROJECT)/pkg/buildinfo.Date=$(BUILD_DATE)

###
### Build
###

# The first target is the default, so a bare `make` builds everything.
.PHONY: build
build: ## Build the Windows executables, checksums.txt and bridge-manifest.json
	@mkdir -p $(DIST)
	for arch in $(BUILD_ARCH); do \
		CGO_ENABLED=0 GOOS=windows GOARCH=$$arch $(GO) build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' \
			-o $(DIST)/$(PROJECT)-windows-$$arch.exe ./cmd/$(PROJECT) || exit 1; \
	done
	$(GO) run ./tools/buildmanifest -dist $(DIST) -version $(VERSION) -commit $(GIT_COMMIT) -date $(BUILD_DATE) -arch '$(BUILD_ARCH)'

###
### Validation
###

.PHONY: fmt
fmt: ## Format code with gofmt
	gofmt -w .

.PHONY: check-fmt
check-fmt: ## Check if code is formatted (for CI)
	@unformatted=$$(gofmt -l .); [ -z "$$unformatted" ] || { echo "The following files are not formatted:"; echo "$$unformatted"; echo "Run 'make fmt' to format the code."; exit 1; }

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: test
test: ## Run the unit tests
	$(GO) test ./...

.PHONY: test-coverage
test-coverage: ## Run the unit tests with a coverage report
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

###
### Cleanup
###

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(DIST) coverage.out coverage.html

###
### Help
###

.PHONY: help
help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
