package private

import "k8s.io/apimachinery/pkg/util/sets"

const (
	GalaxySocketPath = "/var/run/galaxy.sock"
)

var (
	LabelKeyNetworkType                 = "network"
	LabelValueNetworkTypeFloatingIP     = "FLOATINGIP"
	LabelValueNetworkTypeNAT            = "NAT"
	NodeLabelValueNetworkTypeFloatingIP = "floatingip"

	LabelKeyFloatingIP  = "galaxy.io/floatingip"
	LabelValueImmutable = "immutable"
	AnnotationKeyIPInfo = "galaxy.io/ip"

	NetworkTypeOverlay  = NetworkType{String: sets.NewString("", "DEFAULT", LabelValueNetworkTypeNAT), CNIType: "galaxy-flannel"}
	NetworkTypeUnderlay = NetworkType{String: sets.NewString(LabelValueNetworkTypeFloatingIP), CNIType: "galaxy-k8s-vlan"}

	IPAMTypeZhiyun = "galaxy-zhiyun-ipam"
)

type NetworkType struct {
	sets.String
	CNIType string
}
