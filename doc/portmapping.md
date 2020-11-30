# Random port mapping

FEATURE STATE: tkestack/galaxy:v1.0.4 [stable]

NodePort service forwards packets randomly to one of the backend. If pod network is overlay network and you want to
visit each pod's web page from outside the cluster because each pod of the same statefulset has a different page, nodePort
is not capable. You can use [port-forward](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/)
to visit a specific pod's web page, but it is a debug tool, you have to execute the command after pod becoming ready. You
can't visit all pods port immediately after they become ready. And you have to hold on the process until pod terminates.

Another option is [hostPort](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/#support-hostport).
CNI plugin allocates the same host port 9040 on nodes for pods requesting for hostPort regarding the following spec.

```
    spec:
      containers:
      - image: ...
        ports:
          - containerPort: 9040
            hostPort: 9040
```

But you can't run two pods requesting the same host port on the same node. As kubernetes document [suggests](https://kubernetes.io/docs/concepts/configuration/overview/)

> When you bind a Pod to a hostPort, it limits the number of places the Pod can be scheduled, because each <hostIP,
> hostPort, protocol> combination must be unique. If you don't specify the hostIP and protocol explicitly, Kubernetes
> will use 0.0.0.0 as the default hostIP and TCP as the default protocol.

If CNI plugin supports random port mapping just like docker bridge network, then the problem is solved. Galaxy implements
this random port mapping feature.

## Usage

Leave hostport empty and add a `tkestack.io/portmapping: ""` to pod annotation. The following is an example yaml.

```
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: hello
  name: hello
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hello
  template:
    metadata:
      labels:
        app: hello
      annotations:
        tkestack.io/portmapping: ""
    spec:
      containers:
      - image: ...
        ports:
          - containerPort: 9040
```

Galaxy will map a random host port to each container port via iptables and write back the random port back as the value 
of `tkestack.io/portmapping` annotation. The value is a json encoding of `[]Port`. `Port` is the following struct.

```
type Port struct {
    // HostPort > 0
    HostPort int32 `json:"hostPort"`

    ContainerPort int32 `json:"containerPort"`
    // "TCP" or "UDP".
    Protocol string `json:"protocol"`

    HostIP string `json:"hostIP,omitempty"`
 
    PodName string `json:"podName"`
 
    PodIP string `json:"podIP"`
}
```

Please note that galaxy will bind all allocated random host ports to avoid that they are used as random ports to send package.
