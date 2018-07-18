#!/usr/bin/env bash

if [[ ${DEBUG} -gt 0 ]]; then set -x; fi

NETCONFPATH=${NETCONFPATH-/etc/cni/net.d}

function exec_plugins() {
    i=0
    contid=$2
    netns=$3
    export CNI_COMMAND=$(echo $1 | tr '[:lower:]' '[:upper:]')
    export PATH=$CNI_PATH:$PATH
    export CNI_CONTAINERID=$contid
    export CNI_NETNS=$netns
    ipInfo='{"ip":"10.0.2.14/24","vlan":2,"gateway":"10.0.2.1"}'
    ipInfo2='{"ip":"111.230.207.144/24","vlan":3,"gateway":"111.230.207.1"}'
    export CNI_ARGS="K8S_POD_NAME=pod1;IgnoreUnknown=true;IPInfo=$ipInfo;SecondIPInfo=$ipInfo2"
    echo $PATH
    for netconf in $(echo $NETCONFPATH/*.conf | sort); do
        name=$(jq -r '.name' <$netconf)
        plugin=$(jq -r '.type' <$netconf)
        export CNI_IFNAME=$(printf eth%d $i)
                echo $name
                echo $netconf
        res=$($plugin <$netconf)
        if [ $? -ne 0 ]; then
            errmsg=$(echo $res | jq -r '.msg')
            if [ -z "$errmsg" ]; then
                errmsg=$res
            fi

            echo "${name} : error executing $CNI_COMMAND: $errmsg"
            exit 1
        elif [[ ${DEBUG} -gt 0 ]]; then
            echo ${res} | jq -r .
        fi

        let "i=i+1"
    done
}

if [ $# -ne 3 ]; then
    echo "Usage: $0 add|del CONTAINER-ID NETNS-PATH"
    echo "  Adds or deletes the container specified by NETNS-PATH to the networks"
    echo "  specified in \$NETCONFPATH directory"
    exit 1
fi

exec_plugins $1 $2 $3
