# Running Galaxy

Please refer to [building](building.md) for building docker image first.

Copy all files in yaml directory to you kubectl client machine.
And then docker push your image to Docker hub or private registry server.

## Starting Galaxy daemonset

```
# Please update image prefix in these yamls
kubectl create -f galaxy.yaml
```

## Update Kubelet command line args

Add ```--network-plugin=cni --cni-bin-dir=/opt/cni/bin/``` these args to kubelet and restart it.

## Starting flannel

Galaxy defaults to use flannel for creating Overlay network for Pods. Please refer to [flannel](https://github.com/coreos/flannel) to install flannel.

Please refer to [Galaxy configuration](galaxy-config.md) if you don't wand to install flannel and want to change the default network.

## Starting an example deployment

```
cat <<EOF | kubectl create -f -
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
 labels:
   app: nginx
 name: nginx
spec:
 replicas: 2
 selector:
   matchLabels:
     app: nginx
 template:
   metadata:
     labels:
       app: nginx
   spec:
     containers:
     - image: nginx:1.7.9
       name: nginx
EOF
```

Waiting for Pods to be ready and check if you can reach pods by ```curl http://$(pod_ip)```