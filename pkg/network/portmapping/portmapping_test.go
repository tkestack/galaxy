package portmapping

import "testing"

func TestOpenRandomPort(t *testing.T) {
	hp := &hostport{protocol: "tcp"}
	closer, err := openLocalPort(hp)
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if hp.port == 0 {
		t.Fatal()
	}
	hp = &hostport{protocol: "udp"}
	closer, err = openLocalPort(hp)
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if hp.port == 0 {
		t.Fatal()
	}
}
