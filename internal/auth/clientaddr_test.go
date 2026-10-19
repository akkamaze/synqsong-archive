package auth

import (
	"net/netip"
	"testing"
)

func TestClientAddrsGroupIPv4By32AndIPv6By64(t *testing.T) {
	same := [][2]string{
		{"2001:db8:1:2::1", "2001:db8:1:2:ffff:ffff:ffff:ffff"},
		{"::ffff:192.0.2.7", "192.0.2.7"},
		{"fe80::1%eth0", "fe80::1"},
	}

	for _, pair := range same {
		if ip(pair[0]) != ip(pair[1]) {
			t.Errorf("%s and %s are different clients, want one", pair[0], pair[1])
		}
	}

	different := [][2]string{
		{"192.0.2.7", "192.0.2.8"},
		{"2001:db8:1:2::1", "2001:db8:1:3::1"},
	}

	for _, pair := range different {
		if ip(pair[0]) == ip(pair[1]) {
			t.Errorf("%s and %s are one client, want two", pair[0], pair[1])
		}
	}
}

func TestAClientAddrNeedsAnAddress(t *testing.T) {
	if _, err := NewClientAddr(netip.Addr{}); err == nil {
		t.Fatal("an empty address was accepted")
	}
}
