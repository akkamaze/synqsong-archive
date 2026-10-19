// Package room holds the rules of a listening room. It is pure: no I/O, no clock, no JSON.
package room

// Role is someone's standing in one room. The zero value is not a role.
type Role int8

const (
	// RoleGuest listens without an account and can do nothing else.
	RoleGuest Role = iota + 1
	RoleMember
	RoleDJ
	RoleOwner
)

func (r Role) atLeast(min Role) bool {
	return r >= min && r <= RoleOwner
}
