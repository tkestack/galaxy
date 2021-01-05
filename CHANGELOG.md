# v1.0.8

## Feature

- Support reserve ip for all workloads [#107](https://github.com/tkestack/galaxy/pull/107), please check [custom resource workloads](https://github.com/tkestack/galaxy/blob/master/doc/float-ip.md#custom-resource-workloads)
- Support configuring multiple device of vlan cni plugin [#114](https://github.com/tkestack/galaxy/pull/114)

## Bug fix

- Fix veth device name conflict when multi network enabled [#115](https://github.com/tkestack/galaxy/pull/115)
- Disable rp_filter in container [#119](https://github.com/tkestack/galaxy/pull/119)
- Fix release IP API race condition [#120](https://github.com/tkestack/galaxy/pull/120)
- Fix list ip API lists reserved IPs [#121] https://github.com/tkestack/galaxy/pull/121

# v1.0.7

## Feature

- [Support allocating multiple ips](https://github.com/tkestack/galaxy/pull/95), [please check usage docs](https://github.com/tkestack/galaxy/blob/master/doc/float-ip.md#allocate-multiple-ips)
- [Add mtu config in ipvlan and macvlan mode](https://github.com/tkestack/galaxy/pull/108)

## Enhance

- [Fix unbind pod is slow if lots of pod exits at the same time](https://github.com/tkestack/galaxy/pull/99)
- [Check if device is veth before deleting eth0](https://github.com/tkestack/galaxy/pull/93)
- [change log level if bridge-nf-call-iptables module is not installed](https://github.com/tkestack/galaxy/issues/89)
- [Make galaxy pods critical guaranteed scheduling](https://github.com/tkestack/galaxy/pull/102)

## Clean up

- [Remove subnet from floatingip spec](https://github.com/tkestack/galaxy/pull/103)

## Bug fix

- [Fix a race condition that resync released ip due to pod uid missmatch](https://github.com/tkestack/galaxy/pull/109)

# v1.0.6

## Feature

- [Release ip of completed pod](https://github.com/tkestack/galaxy/pull/81)
- [Release ip fully concurrently](https://github.com/tkestack/galaxy/pull/83)
- [Add prometheus metrics: galaxy_ip_counter, galaxy_schedule_latency, galaxy_cloud_provider_latency](https://github.com/tkestack/galaxy/pull/85)
- [K8s-vlan Ipvlan cni plugin supports l2, l3s mode](https://github.com/tkestack/galaxy/pull/91)

## Bug fix

- [Ensure crd created before start list and watch crd](https://github.com/tkestack/galaxy/pull/82)
- [Resync release ip if pod recreates with the same name but different uid](https://github.com/tkestack/galaxy/pull/86)

## Clean up

- [Remove second ipam](https://github.com/tkestack/galaxy/pull/80)

# v1.0.5

## Feature

- [Support floatingip for pod with all kinds of workloads or with no owner reference](https://github.com/tkestack/galaxy/pull/74)

## Bug fix

- [Fix race condition of bind/unbind with cluster provider enabled](https://github.com/tkestack/galaxy/pull/72)
