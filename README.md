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

# Test

## CNI plugin

```
go install -v `pwd`/cni/vlan/vlan.go

# create a config
mkdir -p /etc/cni/net.d
# vlan config
cat >/etc/cni/net.d/10-mynet.conf <<EOF
{
    "name": "mynet",
    "type": "vlan",
    "ipam": {
        "type": "host-local",
        "subnet": "192.168.33.0/24",
        "routes": [
            { "dst": "0.0.0.0/0" }
        ],
        "gateway": "192.168.33.1"
    },
    "device": "eth1",
    "vlan": 3
}
EOF
# optional loop config
cat >/etc/cni/net.d/99-loopback.conf <<EOF
{
    "cniVersion": "0.2.0",
    "type": "loopback"
}
EOF

cd vendor/github.com/containernetworking/cni
./build
CNI_PATH=`pwd`/bin
cd scripts
CNI_PATH=$CNI_PATH CNI_ARGS="IP=192.168.33.3" ./priv-net-run.sh ip ad
```