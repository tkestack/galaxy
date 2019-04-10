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

## API

### List Float IPs

```
curl http://$galaxy-ipam-service/v1/ip?keyword=nginx&page=0&size=10
```

### Release Float IPs

```
curl http://$galaxy-ipam-service/v1/ip -X POST -H "Content-Type: application/json" -d '{"ips":[{"ip":"10.0.0.2","poolName":"pool1"}]}'
```