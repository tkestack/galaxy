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
