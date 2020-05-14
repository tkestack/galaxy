# Tencent is pleased to support the open source community by making TKEStack
# available.
#
# Copyright (C) 2012-2019 Tencent. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use
# this file except in compliance with the License. You may obtain a copy of the
# License at
#
# https://opensource.org/licenses/Apache-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OF ANY KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations under the License.

.PHONY: all
all: build

# ==============================================================================
# Build options

ROOT_PACKAGE=tkestack.io/galaxy
VERSION_PACKAGE=$(ROOT_PACKAGE)/pkg/utils/ldflags

# ==============================================================================
# Includes

include build/lib/common.mk
include build/lib/image.mk
include build/lib/golang.mk
include build/lib/docker-buildx.mk

# ==============================================================================
# Usage

define USAGE_OPTIONS

Options:
  DEBUG        Whether to generate debug symbols. Default is 0.
  BINS         The binaries to build. Default is galaxy and galaxy-ipam.
               Example: make image BINS="galaxy galaxy-ipam"
  PLATFORMS    The multiple platforms to build. Default is linux_amd64 and linux_arm64.
               This option is available when using: make build.multiarch/image.multiarch/push.multiarch
               Example: make image.multiarch BINS="galaxy galaxy-ipam" PLATFORMS="linux_amd64 linux_arm64"
  VERSION      The version information compiled into binaries.
               The default is obtained from VERSION file.
  V            Set to 1 enable verbose build. Default is 0.
endef
export USAGE_OPTIONS

# ==============================================================================
# Targets

## build: Build source code for host arch.
.PHONY: build
build:
	@$(MAKE) go.build

## build.multiarch: Build source code for multiple platforms. See option PLATFORMS.
.PHONY: build.multiarch
build.multiarch:
	@$(MAKE) go.build.multiarch

## clean: Remove all files that are created by building.
.PHONY: clean
clean:
	@$(MAKE) go.clean

## image: Build docker images for host arch.
.PHONY: image
image:
	@$(MAKE) image.build

## image.multiarch: Build docker images for multiple platforms. See option PLATFORMS.
.PHONY: image.multiarch
image.multiarch:
	@$(MAKE) image.build.multiarch

## push: Build docker images for host arch and push images to registry.
.PHONY: push
push:
	@$(MAKE) image.push

## push.multiarch: Build docker images for multiple platforms and push images to registry.
.PHONY: push.multiarch
push.multiarch:
	@$(MAKE) image.push.multiarch

## manifest: Build docker images for host arch and push manifest list to registry.
.PHONY: manifest
manifest:
	@$(MAKE) image.manifest.push

## manifest.multiarch: Build docker images for multiple platforms and push manifest lists to registry.
.PHONY: manifest.multiarch
manifest.multiarch:
	@$(MAKE) docker.push.multiarch

.PHONY: clean.buildx
clean.buildx:
	@$(MAKE) buildx.clean

## test: 
.PHONY: test
test:
	sudo -E env "PATH=${PATH}:$(go env GOPATH)/bin" hack/test.sh

## codegen: Update codegen
.PHONY: codegen
codegen:
	hack/update-codegen.sh

## update: Update vendor
.PHONY: update
update:
	hack/updatevendor.sh

## help: Show this help info.
.PHONY: help
help: Makefile
	@echo -e "\nUsage: make <TARGETS> <OPTIONS> ...\n\nTargets:"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo "$$USAGE_OPTIONS"
