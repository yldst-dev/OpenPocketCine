package network

import (
	"net/netip"
	"testing"
)

func TestHostsExcludeNetworkBroadcastAndSelf(t *testing.T) {
	lan := LAN{Address: netip.MustParseAddr("192.168.10.1"), Prefix: netip.MustParsePrefix("192.168.10.0/30")}
	hosts, err := Hosts(lan)
	if err != nil || len(hosts) != 1 || hosts[0].String() != "192.168.10.2" {
		t.Fatalf("wrong hosts: %v %v", hosts, err)
	}
	lan.Prefix = netip.MustParsePrefix("192.168.0.0/16")
	if _, err := Hosts(lan); err == nil {
		t.Fatal("accepted unbounded scan")
	}
}

func TestPublicAndMalformedTargetsRejected(t *testing.T) {
	for _, target := range []string{"8.8.8.8", "localhost", "127.0.0.1", "::1", "192.168.1.2:9004", "224.0.0.1", ""} {
		if _, err := ParseCamera(target); err == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	if _, err := ParseCamera("192.168.10.2"); err != nil {
		t.Fatal(err)
	}
}
