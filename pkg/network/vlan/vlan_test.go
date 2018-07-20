package vlan

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalVlanNetConf(t *testing.T) {
	var nc NetConf
	if err := json.Unmarshal([]byte("{}"), &nc); err != nil {
		t.Fatal(err)
	}
	if nc.DisableDefaultBridge != nil {
		t.Fatal(*nc.DisableDefaultBridge)
	}
	if err := json.Unmarshal([]byte(`{"disable_default_bridge": true}`), &nc); err != nil {
		t.Fatal(err)
	}
	if nc.DisableDefaultBridge == nil || *nc.DisableDefaultBridge != true {
		t.Fatalf("%v", nc.DisableDefaultBridge)
	}
}
