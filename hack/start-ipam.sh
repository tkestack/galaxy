#! /bin/bash

/usr/bin/galaxy-ipam --logtostderr=false --log-dir=/data/galaxy-ipam/logs --profiling --v=4 --config=/etc/galaxy/galaxy-ipam.json --kubeconfig=/etc/kubernetes/kubelet-kubeconfig --port=80 --leader-elect