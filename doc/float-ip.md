# Float IP

With the help of Galaxy-ipam, Galaxy offers Float IP for PODs of Kubernetes workloads. Float IP has a different meaning for each Release Policy and workload.

Galaxy currently supports Float IP function for Deployment and Statefulsets PODs.

## Usage

Specify extended resource requests and limits `tke.cloud.tencent.com/eni-ip:1` in one of the container definition of POD spec.

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

Galaxy supports three kind of release policy. Add a POD annotation naming `k8s.v1.cni.galaxy.io/release-policy` with the following value:

- `never`, Never Release IP even if the Deployment or Statefulset is deleted. Submitting a same name Deployment or Statefulset will reuse previous reserved IPs. 
- `immutable`, Release IP Only when deleting or scaling down Deployment or Statefulset. If POD float onto a new node in case of original Node became NotReady, it will get the previous IP.

If the annotation is not specified or empty or any other value, the IP will be released once the POD floats or deleted.

## Float IP Pool

Galaxy also supports Deployment IP Pool which shares IPs among several Deployments by setting a `tke.cloud.tencent.com/eni-ip-pool` POD annotation with a given pool name as value.

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

## Rolling upgrade policy issue

Default update strategy for a deployment is `StrategyType=RollingUpdate` and `25% max unavailable, 25% max surge`, this
means during upgrading a deployment, deployment controller when make sure a max of 25% pods beyond replicas will be created
and a max of 25% pods of replicas won't be running. Consider a deployment of replicas <= 3, when upgrading it, deployment
controller will first create a new pod and ensure it become running before teardown an old pod. Issue comes if the deployment
also asks for float IP release policy `immutable` or `never`, which means during upgrading galaxy-ipam will ensure a new
pod gets the same IP from an old pod and thus make new pods waits at scheduling phase for terminating of old pods to get
reuse their IPs.

Thus the two strategy resulting in a dead lock during upgrading for a `replicas <= 3` deployment. We suggest to release
one strategy to get upgrade working.

## API

Galaxy-ipam provides swagger 1.2 docs. Please check [swagger.json](swagger.json) for cached galaxy-ipam API doc.
Also, you can add `--swagger` command line args to galaxy-ipam and restart it, check `http://${galaxy-ipam-ip}:9041/apidocs.json/v1`.

