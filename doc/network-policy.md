# Network policy

## Enable network policy

Add ```--network-policy``` flag to Galaxy and restart it.

## Details

Kubernetes network policy is superset of multi-tenent network which is a namespace level policy, because it accounts to pod level.
Based on this, it may be impossible to implement it by network protocols such as VXLAN.

`Iptables` is suitable for filtering packets based on protocols, ips and ports. But it has a fatal weakness that kernel
travel accross chain rules one by one to determine if packets match them. Consider the selected pods' ips by `namespaceSelector`
and `podSelector` in `ingress.from`, they may be a sparse set, if using iptables, we have to write many rules which is quite ineffient.
This brings `ipset` which uses hash map or bloom filter to match ips or ports. So the basic design is using `iptables` along with `ipset`.

![image](image/policy-ipset.png)

- `ipset` `hash:ip` is used to match `namespaceSelector` and `podSelector`
- `ipset` `hash:net` is used to match `ipBlock`, `ipset` supports nomatch option to except serveral cases
- For `ports` part, we can make same protcol ports a single iptables rule by using multiport iptables extension to match them

The ingress rule may be as follows if there is no `bridge` in your cni network

![image](image/policy-ingress-rule.png)

and egress rule may be

![image](image/policy-egress-rule.png)
