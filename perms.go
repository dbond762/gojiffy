package gojiffy

// Perms — права текущего пользователя. Разграничение доступа идёт по ним,
// а не по названию роли: роль — это просто набор прав.
type Perms map[string]bool

func (p Perms) Can(name string) bool { return p[name] }
