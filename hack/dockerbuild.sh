package=/go/src/git.code.oa.com/gaiastack/galaxy
docker run --rm -v `pwd`:$package golang:1.6.2 go build -o $package/bin/galaxy -v $package/cni/vlan/vlan.go 
