#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

function tar_code() {
    BIND_DIR=bin/${NAME}-${VERSION}
    mkdir -p ${BIND_DIR}
    for i in ${CURDIR}/*; do
      if [ "`basename $i`" != "bin" ]; then
         cp -R $i ${BIND_DIR}
      fi
    done
    mkdir -p ${BIND_DIR}/go/src/github.com/containernetworking
    rm -rf ${BIND_DIR}/go/src/github.com/containernetworking/plugins
    tar zxvf hack/plugins-0.6.0.tar.gz -C ${BIND_DIR}/go/src/github.com/containernetworking/
    mv ${BIND_DIR}/go/src/github.com/containernetworking/plugins-0.6.0 ${BIND_DIR}/go/src/github.com/containernetworking/plugins
    rm -rf ${CURDIR}/bin/${NAME}-${VERSION}.tar.gz
    tar cf ${CURDIR}/bin/${NAME}-${VERSION}.tar -C ${CURDIR}/bin .
    gzip -f ${CURDIR}/bin/${NAME}-${VERSION}.tar
}

function build_rpm() {
    RPMNAME=${NAME}-${VERSION}-${GITCOMMITNUM}.tl2.x86_64.rpm
    docker create -it --name ${NAME} \
        -e GITVERSION=${GITVERSION} \
        -e GITCOMMITNUM=${GITCOMMITNUM} \
        -e VERSION=${VERSION} \
        docker.oa.com:8080/gaia/k8s-builder:latest rpmbuild -bb --clean \
        --define="gitversion ${GITVERSION}" \
        --define="commit ${GITCOMMITNUM}" \
        --define="version ${VERSION}" /root/rpmbuild/SPECS/${NAME}.spec
    docker cp ${CURDIR}/bin/${NAME}-${VERSION}.tar.gz ${NAME}:/root/rpmbuild/SOURCES/
    docker cp ${CURDIR}/hack/config/${NAME}.spec ${NAME}:/root/rpmbuild/SPECS/
    docker start -ai ${NAME}
    docker wait ${NAME}
    docker cp ${NAME}:/root/rpmbuild/RPMS/x86_64/${RPMNAME} bin/
}

function upload() {
    RPMNAME=${NAME}-${VERSION}-${GITCOMMITNUM}.tl2.x86_64.rpm
    RPMFILE=${CURDIR}/bin/${RPMNAME}
    size=$(ls -l ${RPMFILE} | awk '{print $5}')
    curl -v 'http://gaia.repo.oa.com/upload_file?filesize='${size}'&filename='${RPMNAME}'&dirtype=1' -T ${RPMFILE}
    curl -v 'http://gaia.repo.oa.com/update_repo?dirtype=1'
}

trap "cleanup" EXIT SIGINT SIGTERM
function cleanup () {
    rm -rf bin/${NAME}-${VERSION}
    rm -rf bin/*.tar.gz
    docker rm -vf ${NAME} &> /dev/null || true
}

CURDIR=${PWD}
GITCOMMITNUM=$(git log --oneline|wc -l|sed -e 's/^[ \t]*//')
GITVERSION=$(git log --first-parent -1 --oneline | awk '{print $1}')
VERSION=$(git describe --contains --all HEAD | sed -e 's/branch-//')
if [ -n "$BRANCH" ]; then VERSION=$(echo $BRANCH | sed -e 's/branch-//' | sed -e 's/.*\///'); fi

for NAME in galaxy galaxy-ipam; do
    tar_code
    build_rpm
    # we have enable nounset, if DEBUG is un defined, we have to use bash parameter expansion(http://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#tag_18_06_02)
    # to avoid unbound variable error
    if [ -z ${DEBUG+x} ]; then
        upload
    fi
    cleanup
done
