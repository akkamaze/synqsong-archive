package account

// SiteRole is someone's standing on the whole site, apart from their role in any room.
type SiteRole int8

const (
	SiteUser SiteRole = iota + 1
	// SiteAdmin can set someone a new PIN when they forget theirs.
	SiteAdmin
)
