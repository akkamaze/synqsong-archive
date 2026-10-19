// Package room holds the rules of a listening room. It is pure: no I/O, no clock, no JSON.
package room

// Role is a member's standing in one room. The zero value is not a role.
type Role int8

const (
	RoleMember Role = iota + 1
	RoleDJ
	RoleOwner
)

func (r Role) atLeast(min Role) bool {
	return r >= min && r <= RoleOwner
}
