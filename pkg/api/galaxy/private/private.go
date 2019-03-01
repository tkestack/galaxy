package private

import "k8s.io/apimachinery/pkg/util/sets"

const (
	GalaxySocketPath = "/var/run/galaxy.sock"
)

var (
	LabelKeyNetworkType             = "network"
	LabelValueNetworkTypeFloatingIP = "FLOATINGIP"
	LabelValueNetworkTypeNAT        = "NAT"

	LabelKeyFloatingIP     = "galaxy.io/floatingip"
	LabelValueImmutable    = "immutable" // Release IP Only when deleting or scale down App
	LabelValueNeverRelease = "never"     // Never Release IP

	LabelKeyEnableSecondIP = "galaxy.io/secondip"
	LabelValueEnabled      = "true"

	NetworkTypeOverlay  = NetworkType{String: sets.NewString("", "DEFAULT", LabelValueNetworkTypeNAT), CNIType: "galaxy-flannel"}
	NetworkTypeUnderlay = NetworkType{String: sets.NewString(LabelValueNetworkTypeFloatingIP), CNIType: "galaxy-k8s-vlan"}

	CNIBridgePlugin = "galaxy-bridge"

	CNISet = sets.NewString("galaxy-flannel", "galaxy-k8s-vlan")
)

type NetworkType struct {
	sets.String
	CNIType string
}
