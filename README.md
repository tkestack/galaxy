## Galaxy: Providing high-performance network and float IP for Kubernetes workloads
[![Build Status](https://api.travis-ci.com/gaiastackorg/galaxy.svg?branch=master)](https://travis-ci.com/gaiastackorg/galaxy)
[![Codecov branch](https://img.shields.io/codecov/c/github/gaiastackorg/galaxy/master.svg?style=for-the-badge)](https://codecov.io/gh/gaiastackorg/galaxy)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://tkestack.io/galaxy/merge_requests)

Galaxy is a Kubernetes network project designed for providing both common Overlay and high performance Underlay network for pods.
And it also implements float IP(or elastic IP) support, i.e. Pod's IP won't change even if it float onto another node due to node crash, which benefits for running stateful set applications.

Currently, it consists of three components - Galaxy, CNI plugins and Galaxy IPAM.
Galaxy is a daemon process running on each kubelet node which invokes different kinds of CNI plugins to setup the required networks for pods.
Galaxy IPAM is a Kubernetes Scheduler plugin which works as a Float IP configuration and allocation manager.

Also, galaxy is compatible with CNI spec and you can integrate any CNI plugin with galaxy by installing CNI binaries and updating [network configuration](doc/galaxy-config.md).

## Using Galaxy

- [Getting started](doc/getting-started.md)
- [Galaxy configuration](doc/galaxy-config.md)
- [Galaxy-ipam configuration](doc/galaxy-ipam-config.md)
- [Float IP usage](doc/float-ip.md)
- [Supported CNI plugins](doc/supported-cnis.md)
- [Network policy](doc/network-policy.md)

## Contributing

Galaxy is written in Golang like lots of Kubernetes project. Please refer to [install golang](https://golang.org/doc/install) first. If you want to build Galaxy right away, please check [building Galaxy](doc/building.md).

For more information about contributing issues or pull requests, see our [Contributing to Galaxy](doc/contributing.md).

## License

Galaxy is under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.
