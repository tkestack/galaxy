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
# Makefile helper functions for docker image

DOCKER := docker
DOCKER_SUPPORTED_API_VERSION ?= 1.32

REGISTRY_PREFIX ?= tkestack
BASE_IMAGE = centos:7

EXTRA_ARGS ?=
_DOCKER_BUILD_EXTRA_ARGS :=

ifdef HTTP_PROXY
_DOCKER_BUILD_EXTRA_ARGS += --build-arg http_proxy=${HTTP_PROXY}
endif
ifdef HTTPS_PROXY
_DOCKER_BUILD_EXTRA_ARGS += --build-arg https_proxy=${HTTPS_PROXY}
else ifdef HTTP_PROXY
_DOCKER_BUILD_EXTRA_ARGS += --build-arg https_proxy=${HTTP_PROXY}
endif

ifneq ($(EXTRA_ARGS), )
_DOCKER_BUILD_EXTRA_ARGS += $(EXTRA_ARGS)
endif

.PHONY: image.verify
image.verify:
	$(eval API_VERSION := $(shell $(DOCKER) version | grep -E 'API version: {6}[0-9]' | awk '{print $$3} END { if (NR==0) print 0}' ))
	$(eval GREATER := $(shell echo "$(API_VERSION) > $(DOCKER_SUPPORTED_API_VERSION)" | bc))
	@if [ $(GREATER) -ne 1 ]; then \
		$(DOCKER) -v ;\
		echo "Unsupported docker version. Docker API version should be greater than $(DOCKER_SUPPORTED_API_VERSION)"; \
		exit 1; \
	fi

.PHONY: image.daemon.verify
image.daemon.verify: image.verify
	$(eval PASS := $(shell $(DOCKER) version | grep -q -E 'Experimental: {5}true' && echo 0 || echo 1))
	@if [ $(PASS) -ne 0 ]; then \
		echo "Experimental features of Docker daemon is not enabled. Please add \"experimental\": true in '/etc/docker/daemon.json' and then restart Docker daemon."; \
		exit 1; \
	fi

.PHONY: image.build
image.build: image.verify go.build $(addprefix image.build., $(addprefix $(PLATFORM)., $(BINS)))

.PHONY: image.build.multiarch
image.build.multiarch: image.verify  go.build.multiarch \
$(foreach p,$(PLATFORMS),$(addprefix image.build., $(addprefix $(p)., $(BINS))))

.PHONY: image.build.%
image.build.%:
	$(eval PLATFORM := $(word 1,$(subst ., ,$*)))
	$(eval IMAGE := $(word 2,$(subst ., ,$*)))
	$(eval OS := $(word 1,$(subst _, ,$(PLATFORM))))
	$(eval ARCH := $(word 2,$(subst _, ,$(PLATFORM))))
	$(eval IMAGE_PLAT := $(subst _,/,$(PLATFORM)))
	@echo "===========> Building docker image $(IMAGE) $(VERSION) for $(IMAGE_PLAT)"
	@mkdir -p $(TMP_DIR)/$(IMAGE)
	@cat $(ROOT_DIR)/build/docker/$(IMAGE)/Dockerfile\
		| sed "s#BASE_IMAGE#$(BASE_IMAGE)#g" >$(TMP_DIR)/$(IMAGE)/Dockerfile
	@cp -R $(OUTPUT_DIR)/bin-$(ARCH)/* $(TMP_DIR)/$(IMAGE)/
	$(eval BUILD_SUFFIX := $(_DOCKER_BUILD_EXTRA_ARGS) --pull -t $(REGISTRY_PREFIX)/$(IMAGE)-$(ARCH):$(VERSION) $(TMP_DIR)/$(IMAGE))
	@if [ $(shell $(GO) env GOARCH) != $(ARCH) ] ; then \
		$(MAKE) image.daemon.verify ;\
		$(DOCKER) build --platform $(IMAGE_PLAT) $(BUILD_SUFFIX) ; \
	else \
		$(DOCKER) build $(BUILD_SUFFIX) ; \
	fi
	@rm -rf $(TMP_DIR)/$(IMAGE)

.PHONY: image.push
image.push: image.verify go.build $(addprefix image.push., $(addprefix $(PLATFORM)., $(BINS)))

.PHONY: image.push.multiarch
image.push.multiarch: image.verify go.build.multiarch \
$(foreach p,$(PLATFORMS),$(addprefix image.push., $(addprefix $(p)., $(BINS))))

.PHONY: image.push.%
image.push.%: image.build.%
	@echo "===========> Pushing image $(IMAGE) $(VERSION) to $(REGISTRY_PREFIX)"
	$(DOCKER) push $(REGISTRY_PREFIX)/$(IMAGE)-$(ARCH):$(VERSION)

.PHONY: image.manifest.push
image.manifest.push: export DOCKER_CLI_EXPERIMENTAL := enabled
image.manifest.push: image.verify go.build \
$(addprefix image.manifest.push., $(addprefix $(PLATFORM)., $(BINS)))

.PHONY: image.manifest.push.%
image.manifest.push.%: image.push.% image.manifest.remove.%
	@echo "===========> Pushing manifest $(IMAGE) $(VERSION) to $(REGISTRY_PREFIX) and then remove the local manifest list"
	@$(DOCKER) manifest create $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION) \
		$(REGISTRY_PREFIX)/$(IMAGE)-$(ARCH):$(VERSION)
	@$(DOCKER) manifest annotate $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION) \
		$(REGISTRY_PREFIX)/$(IMAGE)-$(ARCH):$(VERSION) \
		--os $(OS) --arch ${ARCH}
	@$(DOCKER) manifest push --purge $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION)

# Docker cli has a bug: https://github.com/docker/cli/issues/954
# If you find your manifests were not updated,
# Please manually delete them in $HOME/.docker/manifests/
# and re-run.
.PHONY: image.manifest.remove.%
image.manifest.remove.%:
	@rm -rf ${HOME}/.docker/manifests/docker.io_$(REGISTRY_PREFIX)_$(IMAGE)-$(VERSION)

.PHONY: image.manifest.push.multiarch
image.manifest.push.multiarch: image.push.multiarch $(addprefix image.manifest.push.multiarch., $(BINS))

.PHONY: image.manifest.push.multiarch.%
image.manifest.push.multiarch.%:
	@echo "===========> Pushing manifest $* $(VERSION) to $(REGISTRY_PREFIX) and then remove the local manifest list"
	REGISTRY_PREFIX=$(REGISTRY_PREFIX) PLATFROMS="$(PLATFORMS)" IMAGE=$* VERSION=$(VERSION) DOCKER_CLI_EXPERIMENTAL=enabled \
	$(ROOT_DIR)/build/lib/create-manifest.sh 
