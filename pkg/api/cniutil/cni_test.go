package cniutil

import (
	"encoding/json"
	"testing"
)

func TestReverse(t *testing.T) {
	infos := []*NetworkInfo{{NetworkType: "2", Args: map[string]string{"2": "2"}}, {NetworkType: "1"}, {NetworkType: "3"}}
	reverse(infos)
	data, err := json.Marshal(infos)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `[{"NetworkType":"3","Args":null,"Conf":null,"IfName":""},{"NetworkType":"1","Args":null,"Conf":null,"IfName":""},{"NetworkType":"2","Args":{"2":"2"},"Conf":null,"IfName":""}]` {
		t.Fatal(string(data))
	}
}
