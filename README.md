# Build

hack/dockerbuild.sh(mac) or hack/build.sh(linux)

# Development

```
git clone --recursive http://git.code.oa.com/gaiastack/galaxy.git

# or
git clone http://git.code.oa.com/gaiastack/galaxy.git
git submodule init
git submodule update
# remove vendor directory of submodules to get compiled by go build
rm -rf vendor/k8s.io/kubernetes/vendor
rm -rf vendor/github.com/containernetworking/cni/vendor
git update-index --assume-unchanged vendor/k8s.io/kubernetes
git update-index --assume-unchanged vendor/github.com/containernetworking/cni
# track the changes again before update a dependency
git update-index --no-assume-unchanged <path/to/file>

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

create a network config
```
mkdir -p /etc/cni/net.d
# vlan config
cat >/etc/cni/net.d/10-mynet.conf <<EOF
{
    "name": "mynet",
    "type": "galaxy-vlan",
    "ipam": {
        "type": "host-local",
        "subnet": "192.168.33.0/24",
        "routes": [
            { "dst": "0.0.0.0/0" }
        ],
        "gateway": "192.168.33.1"
    },
    "device": "eth1"
}
EOF
# optional loop config
cat >/etc/cni/net.d/99-loopback.conf <<EOF
{
    "cniVersion": "0.2.0",
    "type": "loopback"
}
EOF
```

Execute plugin via cni script
```
CNI_PATH=`pwd`/bin
cd vendor/github.com/containernetworking/cni
# cni scripts depends on jq
apt-get install jq
cd scripts
CNI_PATH=$CNI_PATH CNI_ARGS="IP=192.168.33.3" ./priv-net-run.sh ip ad
```

Execute plugin manually
 ```
export PATH=`pwd`/bin
CNI_PATH=`pwd`/bin
ip netns add ctn
CNI_ARGS="IP=192.168.33.3" CNI_COMMAND="ADD" CNI_CONTAINERID=ctn1 CNI_NETNS=/var/run/netns/ctn CNI_IFNAME=eth0 CNI_PATH=$CNI_PATH galaxy-vlan < /etc/cni/net.d/10-mynet.conf
 ```

# Release

hack/build-rpm.sh
