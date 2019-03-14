#! /bin/bash

cp /etc/cni/net.d/000-galaxy.conf /host/etc/cni/net.d/
cp /opt/cni/bin/galaxy-sdn /host/opt/cni/bin/galaxy-sdn
/usr/bin/galaxy --logtostderr=false --log-dir=/data/galaxy/logs --v=3 --kubeconfig=/host/etc/kubernetes/kubelet-kubeconfig --route-eni