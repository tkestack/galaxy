package private

import "k8s.io/apimachinery/pkg/util/sets"

const (
	GalaxySocketPath = "/var/run/galaxy.sock"
)

var (
	LabelKeyNetworkType                 = "network"
	LabelValueNetworkTypeFloatingIP     = "FLOATINGIP"
	NodeLabelValueNetworkTypeFloatingIP = "floatingip"

	LabelKeyFloatingIP  = "galaxy.io/floatingip"
	LabelValueImmutable = "immutable"
	AnnotationKeyIPInfo = "galaxy.io/ip"

	NetworkTypeOverlay  = NetworkType{String: sets.NewString("", "DEFAULT", "NAT"), CNIType: "galaxy-flannel"}
	NetworkTypeUnderlay = NetworkType{String: sets.NewString("FLOATINGIP"), CNIType: "galaxy-k8s-vlan"}

	IPAMTypeZhiyun = "galaxy-zhiyun-ipam"
)

type NetworkType struct {
	sets.String
	CNIType string
}
