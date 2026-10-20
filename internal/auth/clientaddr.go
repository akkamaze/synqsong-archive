package auth

import (
	"errors"
	"net/netip"
)

// ClientAddr is the network a request came from, as sign-in limits count it: one IPv4
// address, or one IPv6 /64, since a single home or phone is usually given a whole /64.
type ClientAddr struct {
	network netip.Prefix
}

var errNoClientAddr = errors.New("client address: missing, so the request cannot be rate limited")

// NewClientAddr groups an address; which header to trust (CF-Connecting-IP from cloudflared) is the web layer's job.
func NewClientAddr(a netip.Addr) (ClientAddr, error) {
	if !a.IsValid() {
		return ClientAddr{}, errNoClientAddr
	}

	a = a.Unmap()

	bits := 64
	if a.Is4() {
		bits = 32
	}

	network, err := a.Prefix(bits)
	if err != nil {
		return ClientAddr{}, errNoClientAddr
	}

	return ClientAddr{network: network}, nil
}

// String is the network in CIDR form, such as 192.0.2.7/32 or 2001:db8:1:2::/64.
func (c ClientAddr) String() string {
	return c.network.String()
}
