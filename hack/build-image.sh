#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

VERSION=$(git describe --contains --all HEAD | sed -e 's/branch-//')
IMAGE=docker.oa.com:8080/gaia/galaxy-ipam:${VERSION}
RPMFILE=$(ls -t bin/galaxy-ipam-*.tl2.x86_64.rpm | head -1)
cat > bin/galay-ipam.dockerfile <<EOF
FROM docker.oa.com:8080/library/tlinux2.2-gaia-with-onion:latest

RUN TZ=PRC && ln -snf /usr/share/zoneinfo/\$TZ /etc/localtime && echo \$TZ > /etc/timezone

RUN rpm --rebuilddb && yum install -y yum-plugin-ovl vim

COPY ${RPMFILE} /galaxy-ipam.tl2.x86_64.rpm

RUN rpm -i /galaxy-ipam.tl2.x86_64.rpm

EXPOSE 9040
EOF
docker build -f bin/galay-ipam.dockerfile -t ${IMAGE} .
if [ -z ${DEBUG+x} ]; then
    docker push ${IMAGE}
fi