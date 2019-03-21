[![Build Status](https://api.travis-ci.com/gaiastackorg/galaxy.svg?branch=master)](https://travis-ci.com/gaiastackorg/galaxy)
[![Codecov branch](https://img.shields.io/codecov/c/github/gaiastackorg/galaxy/master.svg?style=for-the-badge)](https://codecov.io/gh/gaiastackorg/galaxy)

Galaxy is a Kubernetes network project designed for providing both common Overlay and high performance Underlay network for pods.
And it also implements Float IP benefits for running stateful set applications.

Currently, it consists of three components - Galaxy, CNI plugins and Galaxy IPAM.
Galaxy is a daemon process running on each kubelet node which invokes different kinds of CNI plugins to setup the required networks for pods.
Galaxy IPAM is a Kubernetes Scheduler plugin which works as a Float IP configuration and allocation manager.

# Using Galaxy

- [Getting started](doc/getting-started.md)
- [Galaxy configuration](doc/galaxy-config.md)
- [Galaxy-ipam configuration](doc/galaxy-ipam-config.md)
- [Float IP usage](doc/float-ip.md)
- [Supported CNI plugins](doc/supported-cnis.md)
- [Network policy](doc/network-policy.md)
