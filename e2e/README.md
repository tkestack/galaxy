# install ginkgo gomega

install ginkgo, getting start [https://onsi.github.io/ginkgo/](https://onsi.github.io/ginkgo/)

```
go get github.com/onsi/ginkgo/ginkgo
go get github.com/onsi/gomega/...
```

# test

```
cd $project
ginkgo e2e/k8s-vlan

# run a single test with verbose logs
ginkgo -focus "vlan second ips" e2e/k8s-vlan  -- --logtostderr --v=4
```

build and test

```
ginkgo build e2e/k8s-vlan
e2e/k8s-vlan/k8s-vlan.test
```
