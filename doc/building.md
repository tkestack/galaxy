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

# FAQ
Q: Cannot fetch some modules like 'modernc.org/golex', 'modernc.org/xc', etc?
A: The `build.sh` script will call `go mod` command to download these modules from gitlab.com,
   but there are some compatibility problem for `go mod` when resolving gitlab.com addresses, 
   To fix it, add belowing configuration to `~/.gitconfig` :
```
[url "https://gitlab.com/cznic/mathutil.git"]
        insteadOf = https://gitlab.com/cznic/mathutil
[url "https://gitlab.com/cznic/xc.git"]
        insteadOf = https://gitlab.com/cznic/xc
[url "https://gitlab.com/cznic/strutil.git"]
        insteadOf = https://gitlab.com/cznic/strutil
[url "https://gitlab.com/cznic/golex.git"]
        insteadOf = https://gitlab.com/cznic/golex
[url "https://gitlab.com/cznic/cc.git"]
        insteadOf = https://gitlab.com/cznic/cc
```
