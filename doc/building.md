# Build Binaries

```
go get -d tkestack.io/galaxy
cd $GOPATH/src/tkestack.io/galaxy

# On mac
hack/dockerbuild.sh

# on linux
hack/build.sh && hack/build-ipam.sh
```

# Build Docker Image

```
# building Galaxy and all CNI plugins
hack/build-image-galaxy.sh

# builing Galxy-ipam
hack/build-image-ipam.sh
```

# Build RPM

```
hack/build-rpm.sh
```

# Build a single binary

```
BINARY=cmd/galaxy hack/build-single.sh
```
