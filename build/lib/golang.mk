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

# ==============================================================================
# Makefile helper functions for golang
#

GO_IMAGE := golang:1.13.8
GO := go
GO_SUPPORTED_VERSIONS ?= 1.11|1.12|1.13
GO_LDFLAGS += -X $(VERSION_PACKAGE).GIT_COMMIT=$(GIT_COMMIT) \
	-X $(VERSION_PACKAGE).GO_VERSION=$(shell go version | awk '{print $$3}') \
	-X $(VERSION_PACKAGE).BUILD_TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') 

_DOCKER_RUN_EXTRA_ARGS :=

ifdef HTTP_PROXY
_DOCKER_RUN_EXTRA_ARGS += --env HTTP_PROXY=${HTTP_PROXY}
endif
ifdef HTTPS_PROXY
_DOCKER_RUN_EXTRA_ARGS += --env HTTPS_PROXY=${HTTPS_PROXY}
else
_DOCKER_RUN_EXTRA_ARGS += --env HTTPS_PROXY=${HTTP_PROXY}
endif

ifeq ($(ROOT_PACKAGE),)
	$(error the variable ROOT_PACKAGE must be set prior to including golang.mk)
endif

BUILD_SCRIPT := build/lib/build.sh
GOPATH := $(shell go env GOPATH)
ifeq ($(origin GOBIN), undefined)
	GOBIN := $(GOPATH)/bin
endif

.PHONY: go.build.verify
go.build.verify:
ifneq ($(shell $(GO) version | grep -q -E '\bgo($(GO_SUPPORTED_VERSIONS))\b' && echo 0 || echo 1), 0)
	$(error unsupported go version. Please make install one of the following supported version: '$(GO_SUPPORTED_VERSIONS)')
endif
	@$(ROOT_DIR)/$(BUILD_SCRIPT) verify

.PHONY: go.build
go.build: go.build.$(PLATFORM)

.PHONY: go.build.multiarch
go.build.multiarch: $(addprefix go.build., $(PLATFORMS))

.PHONY: go.build.%
go.build.%:
	$(eval ARCH := $(word 2,$(subst _, ,$*)))
	@$(MAKE) go.cni.$(ARCH)
	@if [ $(shell $(GO) env GOOS) != linux ] || [ $(shell $(GO) env GOARCH) != $(ARCH) ]; then \
		$(MAKE) go.docker.build.$*; \
	else \
		$(MAKE) go.local.build.$*; \
	fi

.PHONY: go.cni.%
go.cni.%:
	@echo "===========> Download cni for arch $*"
	@ARCH=$* $(ROOT_DIR)/$(BUILD_SCRIPT) get_cni

.PHONY: go.local.build.%
go.local.build.%: go.build.verify $(addprefix go.entry.build., $(addprefix %., $(BINS)))
	@echo "===========> End of building binaries"

.PHONY: go.entry.build.%
go.entry.build.%:
	$(eval COMMAND := $(word 2,$(subst ., ,$*)))
	$(eval PLATFORM := $(word 1,$(subst ., ,$*)))
	$(eval ARCH := $(word 2,$(subst _, ,$(PLATFORM))))
	@echo "===========> Building binary $(COMMAND) $(VERSION) for $(ARCH)"
	@ARCH=$(ARCH) GO_LDFLAGS="$(GO_LDFLAGS)" $(ROOT_DIR)/$(BUILD_SCRIPT) "$(COMMAND)"

.PHONY: go.docker.build.%
go.docker.build.%: image.daemon.verify
	$(eval IMAGE_PLAT := $(subst _,/,$*))
	@echo "===========> Building binary via $(IMAGE_PLAT) docker container"
	$(DOCKER) run --rm --platform $(IMAGE_PLAT) $(_DOCKER_RUN_EXTRA_ARGS) \
	-v $(ROOT_DIR):/go/src/$(ROOT_PACKAGE) \
	-v $(GOPATH)/pkg:/go/pkg -w /go/src/$(ROOT_PACKAGE) \
	$(GO_IMAGE) /bin/sh -c 'PLATFORM=$* VERSION=$(VERSION) BINS="$(BINS)" make build'
	@## Docker has a bug here. So client may remove image first and tag image
	@$(DOCKER) tag $(GO_IMAGE) $(GO_IMAGE)-$*
	@$(DOCKER) rmi $(GO_IMAGE)

.PHONY: go.clean
go.clean:
	@echo "===========> Cleaning all build output"
	@rm -rf $(OUTPUT_DIR)
