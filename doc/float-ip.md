# Float IP

With the help of Galaxy-ipam, Galaxy offers Float IP for PODs of Kubernetes workloads. Float IP has a different meaning for each Release Policy and workload.

Galaxy currently supports Float IP function for Deployment and Statefulsets PODs.

## Usage

Specify extended resource requests and limits `tke.cloud.tencent.com/eni-ip:1` in one of the container definition of pod spec.

```
...
     containers:
     - image: nginx:1.7.9
       resources:
         limits:
           tke.cloud.tencent.com/eni-ip: "1"
         requests:
           tke.cloud.tencent.com/eni-ip: "1"
```

## Release Policy

Galaxy supports three kind of release policy. Add a pod annotation naming `k8s.v1.cni.galaxy.io/release-policy` with the following value:

- the annotation is not specified or has empty value, release IP once the pod is deleted or finished.
- `immutable`, release IP only when deleting or scaling down deployment or statefulset. If pod floats onto a new node in
case of the original Node became NotReady, it will get the previous IP.
- `never`, never release IP even if deployment or statefulset is deleted. Submitting a deployment or statefulset with
the same name will reuse previous reserved IPs. 

### Custom resource workloads

FEATURE STATE: tkestack/galaxy-ipam:v1.0.8 [alpha]

Before v1.0.8 galaxy-ipam only supports immutable and never release policy for statefulset, deployment and [tapp](https://github.com/tkestack/tapp).
The usage is the same as before. Galaxy-ipam makes use of [dynamic client](https://github.com/kubernetes/client-go/tree/master/dynamic) to watch all custom resource workloads by demand to get
their replicas and decide when to release ips. If you want to use it, please make sure to add list and watch permissions of custom resource to galaxy-ipam cluster role like the following tapp resource.

```
- apiGroups: ["apps.tkestack.io"]
  resources:
  - tapps
  verbs: ["list", "watch"]
```

For pods with never release policy, galaxy-ipam checks if its name matches regular expression `.*-[0-9]*$`, if it does galaxy-ipam binds an IP to a key `$kind_$namespace_$appName_$podName`, otherwise it throws an error of not
supporting never release policy for it. Galaxy-ipam assumes the workload controller generates pod names by workload name and a `-[0-9]*$` suffix like what statefulset does.

For pods that need an immutable strategy, not only its name must satisfy the same regular expression, but also its CustomResourceDefinition must support scale sub-resource and define the [SpecReplicasPath](https://github.com/kubernetes/kubernetes/blob/v1.19.4/staging/src/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1/types.go#L455).
Galaxy-ipam gets the parent workload replicas by `SpecReplicasPath` and invokes `unstructured.NestedInt64` to get the actual replicas to check whether to release IP.

## Float IP Pool

Galaxy also supports Deployment IP Pool which shares IPs among several Deployments by setting a `tke.cloud.tencent.com/eni-ip-pool` pod annotation with a given pool name as value.

Note that Float IP Pool Deployment is always `never` release policy regardless of the value of `k8s.v1.cni.galaxy.io/release-policy`.

### Pool size

By default, pool size grows as replicas of deployment or statefulset grows. While users can also specify a pool size to limit the ips allocated to
the pool which benefits upgrading deployments without changing IPs for blue-green release.

Creating a pool size can either by creating a pool CRD or by HTTP API. This is a example of creating pool size by CRD

```
apiVersion: galaxy.k8s.io/v1alpha1
kind: Pool
metadata:
  name: example-pool
size: 4
```

### Pre-allocate IP for a pool

Galaxy-ipam supports pre-allocating IPs for a pool by setting `preAllocateIP=true` when creating or updating pool via HTTP API. Note that this is not working by creating pool via kubectl.

## Allocate multiple IPs

FEATURE STATE: tkestack/galaxy-ipam:v1.0.7 [alpha]

Add an annotation `k8s.v1.cni.galaxy.io/args: '{"request_ip_range":[["10.0.0.200~10.0.0.238"],["10.0.0.2~10.0.0.30"],["10.0.0.60~10.0.0.80"]]}'` to pod spec.

galaxy-ipam allocates 3 IPs, one from `["10.0.0.200~10.0.0.238"]`, one from `["10.0.0.2~10.0.0.30"]`, one from `["10.0.0.60~10.0.0.80"]`
and write it back to the same annotation `k8s.v1.cni.galaxy.io/args` value.

```
{
	"request_ip_range": [
		["10.0.0.200~10.0.0.238"],
		["10.0.0.2~10.0.0.30"],
		["10.0.0.60~10.0.0.80"]
	],
	"common": {
		"ipinfos": [{
			"ip": "10.0.0.200/24",
			"vlan": 0,
			"gateway": "10.0.0.1"
		}, {
			"ip": "10.0.0.2/24",
			"vlan": 0,
			"gateway": "10.0.0.1"
		}, {
			"ip": "10.0.0.60/24",
			"vlan": 0,
			"gateway": "10.0.0.1"
		}]
	}
}
```

CNI plugins should read the `k8s.v1.cni.galaxy.io/args` annotation value and configuring multiple IPs for pods.
Currently supporting CNI plugins are galaxy-underlay-veth and galaxy-k8s-vlan.

## API

Galaxy-ipam provides swagger 1.2 docs. Please check [swagger.json](swagger.json) for cached galaxy-ipam API doc.
Also, you can add `--swagger` command line args to galaxy-ipam and restart it, check `http://${galaxy-ipam-ip}:9041/apidocs.json/v1`.

### API examples

1. Query ips allocated to a given statefulset

```
curl 'http://192.168.30.7:9041/v1/ip?appName=sts&appType=statefulsets&namespace=default'{
 "last": true,
 "totalElements": 2,
 "totalPages": 1,
 "first": true,
 "numberOfElements": 2,
 "size": 10,
 "number": 0,
 "content": [
  {
   "ip": "10.0.0.112",
   "namespace": "default",
   "appName": "sts",
   "podName": "sts-0",
   "policy": 2,
   "appType": "statefulset",
   "updateTime": "2020-05-29T11:11:44.633383558Z",
   "status": "Deleted",
   "releasable": true
  },
  {
   "ip": "10.0.0.174",
   "namespace": "default",
   "appName": "sts",
   "podName": "sts-1",
   "policy": 2,
   "appType": "statefulset",
   "updateTime": "2020-05-29T11:11:45.132450117Z",
   "status": "Deleted",
   "releasable": true
  }
 ]
}
```

2. Release ip.

```
curl -X POST -H "Content-type: application/json" -d '{"ips":[{"ip":"10.0.0.112", "appName":"sts", "appType":"statefulset", "podName":"sts-0","namespace":"default"},{"ip":"10.0.0.174", "appName":"sts", "appType":"statefulset", "podName":"sts-1", "namespace":"default"}]}' 'http://192.168.30.7:9041/v1/ip'
{
 "code": 200,
 "message": ""
}
```

## FAQ

### Rolling upgrade policy issue

Default update strategy for a deployment is `StrategyType=RollingUpdate` and `25% max unavailable, 25% max surge`, this
means during upgrading a deployment, deployment controller when make sure a max of 25% pods beyond replicas will be created
and a max of 25% pods of replicas won't be running. Consider a deployment of replicas <= 3, when upgrading it, deployment
controller will first create a new pod and ensure it become running before teardown an old pod. Issue comes if the deployment
also asks for float IP release policy `immutable` or `never`, which means during upgrading galaxy-ipam will ensure a new
pod gets the same IP from an old pod and thus make new pods waits at scheduling phase for terminating of old pods to get
reuse their IPs.

Thus the two strategy resulting in a dead lock during upgrading for a `replicas <= 3` deployment. We suggest to release
one strategy to get upgrade working.
