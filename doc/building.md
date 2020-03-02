# Build Binaries

```
go get -d tkestack.io/galaxy
cd $GOPATH/src/tkestack.io/galaxy

# building all binaries
make
# OR
make BINS="galxy galxy-ipam"

# building galxy-ipam
make BINS="galxy-ipam"
```

# Build Docker Image

```
# building all images
make image

# builing Galxy-ipam
make image BINS="galxy-ipam"
```

# Build Docker Image for specified arch

```
# building all images for linux_arm64
make image.multiarch PLATFORMS="linux_arm64"
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
