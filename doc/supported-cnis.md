# Supported CNIs

Galaxy supports any standard [CNI](https://github.com/containernetworking/cni) plugin. You can use it as a CNI framework just like [multus-cni](https://github.com/intel/multus-cni).

While galaxy do has following builtin CNIs.

## SDN CNI

Kubelet launches SDN CNI process according to [CNI configuration](../yaml/galaxy.yaml).

SDN CNI process calls Galaxy daemon via unix socket with all args from Kubelet.

## Veth CNI

Veth CNI is a overlay network plugin which creates a veth pair to connect host network namespace with container.

Veth CNI gets POD IPs from ipam CNI plugin.

## Vlan CNI

Vlan CNI is a underlay network plugin which creates a veth pair to connect host network namespace with container and bridge/macvlan/ipvlan
device to connect host veth pair with host eth device.

Vlan CNI supports configuring vlan for POD IPs.

Vlan CNI gets POD IPs either from CNI Args `ipinfos=[{"ip":"192.168.0.68/26","vlan":2,"gateway":"192.168.0.65"}]` or ipam CNI plugin.

## SRIOV CNI

SRIOV CNI is a underlay network plugin which makes use of SR-IOV on Ethernet Server Adapters. It allocates a VF device and puts it into
container network namespace.

You can check if your intel card supports SR-IOV via [FAQ](https://www.intel.com/content/www/us/en/support/articles/000005722/network-and-i-o/ethernet-products.html)

SRIOV CNI gets POD IPs either from CNI Args `ipinfos=[{"ip":"192.168.0.68/26","vlan":2,"gateway":"192.168.0.65"}]` or ipam CNI plugin.

## TKE route ENI CNI

TKE route ENI CNI is a [Tencent cloud ENI](https://cloud.tencent.com/product/eni) network plugin which creates a veth pair to connect host network namespace with container
and policy routing on host to connect host veth pair with ENI eth device.

TKE route ENI CNI gets POD IPs either from CNI Args `ipinfos=[{"ip":"192.168.0.68/26","vlan":2,"gateway":"192.168.0.65"}]` or ipam CNI plugin.
