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

DOCKER := DOCKER_CLI_EXPERIMENTAL=enabled docker
DOCKER_SUPPORTED_API_VERSION ?= 1.40
DOCKER_VERSION ?= 19.03

REGISTRY_PREFIX ?= tkestack
BUILDER_NAME ?= tkestack-builder

EXTRA_ARGS ?=
_DOCKER_BUILD_EXTRA_ARGS :=

ifdef HTTP_PROXY
_DOCKER_BUILD_EXTRA_ARGS += --build-arg HTTP_PROXY=${HTTP_PROXY}
endif
ifdef HTTPS_PROXY
_DOCKER_BUILD_EXTRA_ARGS += --build-arg https_proxy=${HTTPS_PROXY}
endif

ifneq ($(EXTRA_ARGS), )
_DOCKER_BUILD_EXTRA_ARGS += $(EXTRA_ARGS)
endif

IMAGE_PLATS ?= linux/amd64,linux/arm64

.PHONY: docker.verify
docker.verify:
	$(eval API_VERSION := $(shell $(DOCKER) version | grep -E 'API version: {1,6}[0-9]' | head -n1 | awk '{print $$3} END { if (NR==0) print 0}' ))
	$(eval PASS := $(shell echo "$(API_VERSION) >= $(DOCKER_SUPPORTED_API_VERSION)" | bc))
	@if [ $(PASS) -ne 1 ]; then \
		$(DOCKER) -v ;\
		echo "Unsupported docker version. Docker API version should be greater than $(DOCKER_SUPPORTED_API_VERSION) (Or docker version: $(DOCKER_VERSION))"; \
		exit 1; \
	fi

.PHONY: buildx.create
buildx.create: docker.verify
	$(DOCKER) buildx create --name $(BUILDER_NAME) --driver docker-container --driver-opt network=host --use || true
	$(DOCKER) buildx inspect --bootstrap

.PHONY: docker.buildx.%
docker.buildx.%: docker.verify buildx.create
	$(eval IMAGE := $*)
	$(eval IMAGE_NAME := $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION))
	@echo "===========> Building docker image $(IMAGE) $(VERSION) for $(IMAGE_PLATS) and push to registry"
	$(DOCKER) buildx build --platform $(IMAGE_PLATS) --push -t $(IMAGE_NAME) $(_DOCKER_BUILD_EXTRA_ARGS) \
	 -f $(ROOT_DIR)/build/docker/$(IMAGE)/Dockerfile-buildx $(ROOT_DIR)

.PHONY: docker.push.multiarch
docker.push.multiarch: $(addprefix docker.buildx., $(BINS))

.PHONY: buildx.clean
buildx.clean:
	$(DOCKER) buildx rm $(BUILDER_NAME)
