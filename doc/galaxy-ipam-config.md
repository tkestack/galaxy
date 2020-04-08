# Galaxy-ipam Configuration

Galaxy-ipam is a [Kubernetes Scheudler Extender](https://kubernetes.io/docs/concepts/extend-kubernetes/extend-cluster/#scheduler-extensions).
Scheduler calls Galaxy-ipam for filtering and binding via HTTP, so we need to create a scheduler policy configuration.

## Kubernetes Scheduler Configuration

Because of [https://github.com/kubernetes/kubernetes/pull/59363](https://github.com/kubernetes/kubernetes/pull/59363) (released in 1.10),
we don't need to configure predicates/priorities in policy config, scheduler applies built-in default sets of predicate/prioritizer on pod scheduling.

```
# Creating scheduler Policy ConfigMap
cat <<EOF | kubectl create -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: scheduler-policy
  namespace: kube-system
data:
  policy.cfg: |
    {
      "kind": "Policy",
      "apiVersion": "v1",
      "extenders": [
        {
          "urlPrefix": "http://127.0.0.1:9040/v1",
          "httpTimeout": 10000000000,
          "filterVerb": "filter",
          "BindVerb": "bind",
          "weight": 1,
          "enableHttps": false,
          "managedResources": [
            {
              "name": "tke.cloud.tencent.com/eni-ip",
              "ignoredByScheduler": false
            }
          ]
        }
      ]
    }
EOF

# Add the following config to kube-scheduler and restart it
--policy-configmap=scheduler-policy
```

Note:
If you want to limit each node's max float IPs, please set ignoredByScheduler to false, then the float IP resource will be judge by scheduler's PodFitsResource algorithm.

## Galaxy-ipam Configuration

Galaxy uses CRD to persist allocated IPs.

```
  galaxy-ipam.json: |
    {
      "schedule_plugin": {
        "storageDriver": "k8s-crd",
        "cloudProviderGrpcAddr": "127.0.0.2:80"
      }
    }
```

## float IP Configuration

If running on bare metal environment, please create a ConfigMap floatingip-config.

```
kind: ConfigMap
apiVersion: v1
metadata:
 name: floatingip-config
 namespace: kube-system
data:
 floatingips: '[{"nodeSubnets":["10.0.0.0/16"],"ips":["10.0.70.2~10.0.70.241"],"subnet":"10.0.70.0/24","gateway":"10.0.70.1"}]'
```

- nodeSubnets: the node cidr.
- ips: available POD ips, be sure these IPs are reachable within the node cidr.
- subnet: the POD IP subnet.
- vlan: the POD IP vlan id. If POD IPs are not belongs to the same vlan as node IP, please specify the POD IP vlan ids. Leave it empty if not required.

For a more complex configuration, please take a look at [test_helper.go](pkg/ipam/utils/test_helper.go)

## CNI network configuration

You can use [Vlan CNI or TKE route ENI CNI plugin](supported-cnis.md) to launch float IP Pods. Make sure to update `DefaultNetworks` to `galaxy-k8s-vlan` of galaxy-etc ConfigMap or add `k8s.v1.cni.cncf.io/networks=galaxy-k8s-vlan` annotation to Pod spec.

## Cloud Provider

If running on public or private clouds, Galaxy leverage ENI feature to provide float IPs for PODs.
Please update `cloudProviderGrpcAddr` in galaxy-ipam-etc ConfigMap.

Cloud provider is responsible for

1. Creating and binding ENI for each kubelet node
1. Provide float IP configuration for Galaxy-ipam
1. Implement a GRPC server based on the [ip_provider.proto](../pkg/ipam/cloudprovider/rpc/ip_provider.proto)
1. Update Node status to add [float IP extend resource](float-ip.md) numbers if requiring to limit each node's max float IPs.

# How Galaxy-ipam works

![How galaxy-ipam works](image/galaxy-ipam.png)

This is how Galaxy-ipam supports running underlay network.

1. On private cloud the cluster administrator needs to config the floatingip-config ConfigMap. While on public cloud Cloud
provider should provide that for Galaxy-ipam
1. Kubernetes scheduler calls Galaxy-ipam on filter/priority/bind method
1. Galaxy-ipam checks if POD has a reserved IP, if it does, Galaxy-ipam marks only the nodes within the available subnets of this IP as
valid node, otherwise all nodes that has float IP left. During binding, Galaxy-ipam allocates an IP and writes it into POD annotation.
1. On public cloud, scheduler plugin calls cloud provider to Assign and UnAssign ENI IP.
1. Galaxy gets IP from POD annotation and calls CNIs with them as CNI args.

![How galaxy ipam allocates IP according to network typology](image/galaxy-ipam-scheduling-process.png)

The above picture shows how Galaxy-ipam allocates IP according to the network typology. Floatingip-config ConfigMap has
the information of network typology that which node subnets POD IPs can be placed in. All Galaxy-ipam needs to do is to
follow the rules, so the following is the scheduling process.

1. Scheduler sends all nodes that fit all filter plugins of itself and the POD to be scheduled to Galaxy-ipam.
1. Galaxy-ipam queries its memory table to find node subnets with full pod name matching the to be scheduled POD. 
If not, it finds those with pods' app full name matching. And again if not, it finds all from unallocated IPs. Then it 
matches all nodes' IPs with these node subnetes and return the matched nodes to Scheduler.
1. Scheduler priorities these nodes and picks the top one and calls Galaxy-ipam for binding.
1. Galaxy-ipam updates its memory table to reuse or allocate an IP from the picked node's corresponding subnet. It then
invokes cloud provider to assign the ENI IP. And finally stores the IP in floatip crd.
1. Galaxy-ipam has a memory table and a floatip crd storage for storing allocated IPs. When restarting, it allocates all
IPs in memory by reading floatip crds. It watches POD delete event from Apiserver to release IPs and runs a regular
goroutine to release IPs that should be done but somehow haven't done.

There is something more that worth talking about.

1. For deployment, in order to allocate the same IPs for its' PODs when rolling upgrade. Galaxy-ipam updates keys of
allocated IPs from POD full name to app full name to reserve these IPs. e.g. from `dp_$namespace_$deploymentName_$podName` to `dp_$namespace_$deploymentName_`.
`dp` is short for `deployment`. Then it updates keys of reserved IPs to new Pods by updating keys from app full name to POD full name.
1. Since Scheduler starts a next loop of scheduling without waiting for the previous binding process to finish, for deployment pod,
Galaxy-ipam allocates reused IP at filtering stage instead of binding stage to make sure IPs don't over allocate for a node subnet.
