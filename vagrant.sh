#!/usr/bin/env bash

set -e

cur_dir=`pwd`
export package=${cur_dir#*$GOPATH/src/}
export go_folder=${GO_FOLDER:-/tmp/golang}


function download_go() {
    if [ ! -f $go_folder/go1.10.3.linux-amd64.tar.gz ]; then
        echo golang package not exist in $go_floder, will download
        mkdir -p $go_folder
        wget https://dl.google.com/go/go1.10.3.linux-amd64.tar.gz -O $go_folder/go1.10.3.linux-amd64.tar.gz
    fi
}

action=${1}

case $action in
    up)
        download_go
        vagrant up
        ;;
    destroy|halt|resume|ssh|suspend|status|validate)
        vagrant $action
        ;;
    *)
        echo "Usage: GO_FOLDER=xxx $0 [vagrant_action]"
        echo "    GO_FOLDER is folder where go1.10.3.linux-amd64.tar.gz exist"
        ;;
esac
