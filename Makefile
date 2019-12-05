.PHONY: all
all:
	hack/build.sh verify binary_galaxy binary_ipam

.PHONY: test
test:
	sudo -E env "PATH=${PATH}:$(go env GOPATH)/bin" hack/test.sh

.PHONY: image
image: all
	hack/build.sh image_galaxy image_ipam

.PHONY: codegen
codegen:
	hack/update-codegen.sh

.PHONY: update
update:
	hack/updatevendor.sh
