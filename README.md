# Build

```
hack/dockerbuild.sh
```

# Development

```
git clone --recursive http://git.code.oa.com/gaiastack/galaxy.git

# or
git clone http://git.code.oa.com/gaiastack/galaxy.git
git submodule init
git submodule update

# manage dependencies
go get github.com/kovetskiy/manul
# show dependencies
manul -Q 
# install new new dependencies
manul -I
# remove git submodules for specified/all dependencies
manul -R
# update a dependency
cd vendor/github.com/containernetworking/cni
git checkout $commit
cd -
git commit -am "Update ..."
```

