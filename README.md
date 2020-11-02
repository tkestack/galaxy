## Galaxy: providing high-performance network and float IP for Kubernetes workloads

[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://tkestack.io/galaxy/merge_requests)

Galaxy is a Kubernetes network project designed for providing both overlay and high performance underlay network for pods.
And it also implements float IP(or elastic IP), i.e. Pod's IP won't change even if it float onto another node due to node crash, which benefits for running stateful set applications.

Currently, it consists of three components - Galaxy, CNI plugins and Galaxy IPAM.
Galaxy is a daemon process running on each kubelet node which invokes different kinds of CNI plugins to setup the required networks for pods.
Galaxy IPAM is a Kubernetes Scheduler plugin which works as a float IP configuration and allocation manager.

Galaxy is compatible with CNI spec and you can integrate any CNI plugin with it by installing CNI binaries and updating [network configuration](doc/galaxy-config.md).

## Using Galaxy

- [Getting started](doc/getting-started.md)
- [Galaxy configuration](doc/galaxy-config.md)
- [Galaxy-ipam configuration](doc/galaxy-ipam-config.md)
- [Float IP usage](doc/float-ip.md)
- [Built-in CNI plugins](doc/supported-cnis.md)
- [Network policy](doc/network-policy.md)

## Contributing

Galaxy is written in Golang like lots of Kubernetes project. Please refer to [install golang](https://golang.org/doc/install) first. If you want to build Galaxy right away, please check [building Galaxy](doc/building.md).

For more information about contributing issues or pull requests, see our [Contributing to Galaxy](doc/contributing.md).

## License

Galaxy is under the Apache License 2.0. See the [License](LICENSE) file for details.
