package gojiffy

// Perms — permissions of the current user. Access is granted by these and not
// by the name of a role: a role is just a set of permissions.
type Perms map[string]bool

// Can reports whether the user has the permission name.
func (p Perms) Can(name string) bool { return p[name] }
