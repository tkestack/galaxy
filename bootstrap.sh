#!/usr/bin/env bash

export http_proxy=http://proxy.tencent.com:8080
export https_proxy=http://proxy.tencent.com:8080
echo "export http_proxy=http://proxy.tencent.com:8080" >> /root/.bashrc
echo "export https_proxy=http://proxy.tencent.com:8080" >> /root/.bashrc
echo "export no_proxy=localhost,127.0.0.1,.oa.com" >> /root/.bashrc
echo "export GOPATH=/root/go" >> /root/.bashrc
echo "export PATH=$PATH:/usr/local/go/bin" >> /root/.bashrc
source /root/.bashrc
tar -C /usr/local -xzf /vagrant_go/go1.10.3.linux-amd64.tar.gz
echo "deb http://cz.archive.ubuntu.com/ubuntu xenial main universe" >> /etc/apt/sources.list

apt-get update
apt install -y jq gcc
