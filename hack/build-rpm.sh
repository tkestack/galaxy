#!/usr/bin/env bash

set -e
set -x

if [ -z "$V" ]; then V="2"; fi
CURDIR=${PWD}
CONTAINER_NAME=galaxy
GITCOMMITNUM=$(git log --oneline|wc -l|sed -e 's/^[ \t]*//')
GITVERSION=$V.0
VERSION=${GITVERSION}.2
NAME=galaxy
RPMNAME=${NAME}-${VERSION}-${GITCOMMITNUM}.tl2.x86_64.rpm
RPMFILE=${CURDIR}/bin/x86_64/${RPMNAME}
BIND_DIR=bin/${NAME}-${VERSION}

mkdir -p ${BIND_DIR}
for i in ${CURDIR}/*; do
  if [ "`basename $i`" != "bin" ]; then
     cp -R $i ${BIND_DIR}
  fi
done
rm -rf ${CURDIR}/bin/${NAME}-${VERSION}.tar.gz
tar cf ${CURDIR}/bin/${NAME}-${VERSION}.tar -C ${CURDIR}/bin .
gzip -f ${CURDIR}/bin/${NAME}-${VERSION}.tar
trap "cleanup" EXIT SIGINT SIGTERM
function cleanup () {
    rm -rf ${BIND_DIR}
    docker rm -vf ${CONTAINER_NAME}
}
docker create -it --name ${CONTAINER_NAME} -v ${CURDIR}/bin:/root/rpmbuild/RPMS \
    -e GITVERSION=${GITVERSION} \
    -e GITCOMMITNUM=${GITCOMMITNUM} \
    -e VERSION=${VERSION} \
    docker.oa.com:8080/gaia/rpmbuilder:1.11 rpmbuild -bb --clean \
    --define="gitversion ${GITVERSION}" \
    --define="commit ${GITCOMMITNUM}" \
    --define="version ${VERSION}" /root/rpmbuild/SPECS/galaxy.spec
docker cp ${CURDIR}/bin/${NAME}-${VERSION}.tar.gz ${CONTAINER_NAME}:/root/rpmbuild/SOURCES/
docker cp ${CURDIR}/hack/v${V}/galaxy.spec ${CONTAINER_NAME}:/root/rpmbuild/SPECS/
docker start -ai ${CONTAINER_NAME}
size=$(ls -l ${CURDIR}/bin/x86_64/${RPMNAME} | awk '{print $5}')
#curl -v 'http://gaia.repo.oa.com/upload_file?filesize='${size}'&filename='${RPMNAME}'&dirtype=1' -T ${RPMFILE}
#curl -v 'http://gaia.repo.oa.com/update_repo?dirtype=1'