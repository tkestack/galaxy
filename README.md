[![Build Status](https://api.travis-ci.com/gaiastackorg/galaxy.svg?branch=master)](https://travis-ci.com/gaiastackorg/galaxy)

# Build

hack/dockerbuild.sh(mac) or hack/build.sh(linux)

# Development

Galaxy uses [glide](https://github.com/Masterminds/glide)/[glide-vc](https://github.com/sgotti/glide-vc) to manager vendors

Install glide/glide-vc

```
curl https://glide.sh/get | sh
go get github.com/sgotti/glide-vc
```

Add/Update vendor

```
hack/updatevendor.sh

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
CNI_PATH=$CNI_PATH CNI_ARGS="IPInfo={"ip":"192.168.0.68/26","vlan":0,"gateway":"192.168.0.65","routable_subnet":"192.168.0.64/26"}" ./priv-net-run.sh ip ad
```

Execute plugin manually
 ```
export PATH=`pwd`/bin
CNI_PATH=`pwd`/bin
ip netns add ctn
CNI_ARGS="IPInfo={"ip":"192.168.0.68/26","vlan":0,"gateway":"192.168.0.65","routable_subnet":"192.168.0.64/26"}" CNI_COMMAND="ADD" CNI_CONTAINERID=ctn1 CNI_NETNS=/var/run/netns/ctn CNI_IFNAME=eth0 CNI_PATH=$CNI_PATH galaxy-vlan < /etc/cni/net.d/10-mynet.conf
 ```

## Creating Floating IP Pod

Deployment

```
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: rami
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: rami
        network: FLOATINGIP
    spec:
      containers:
      - name: hello
        image: docker.oa.com:8080/public/2048:onion
        resources:
          requests:
            galaxy.io/floatingip: 1
            memory: 30Mi
            cpu: 100m
```
TApp

```
apiVersion: gaia/v1alpha1
kind: TApp
metadata:
  name: ramitapp
  labels:
    app: ramitapp
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: ramitapp
        network: FLOATINGIP
        galaxy.io/secondip: true
        galaxy.io/floatingip: immutable
    spec:
      containers:
      - name: hello
        image: docker.oa.com:8080/public/2048:onion
        resources:
          request:
            galaxy.io/floatingip: 1
            memory: 20M
            cpu: 200m
```

Add the following field in pod spec to create floating ip pod

- Labels: {"network": "FLOATINGIP"}
- Resource requests: {"galaxy.io/floatingip": 1}

Besides the following is optional.

labels | meaning
-------|--------
galaxy.io/secondip: true | Pod wants two floatingips, the second floatingip will be the default route in container.
galaxy.io/floatingip: immutable | If pod is evicted, its floating ip is preserved. Supports only TApp.

# Release

hack/build-rpm.sh

# Document

- [Design](doc/design.md)
- [Floating IP申请](doc/ip.md)
- [Floating IP配置](doc/configip.md)
