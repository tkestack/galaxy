#! /bin/bash

cp /etc/cni/net.d/00-galaxy.conf /host/etc/cni/net.d/
cp /opt/cni/bin/* /host/opt/cni/bin/
/usr/bin/galaxy --logtostderr=false --log-dir=/data/galaxy/logs --v=3 --kubeconfig=/host/etc/kubernetes/kubelet-kubeconfig --route-eni