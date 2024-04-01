package models

import "errors"

type Role struct {
	slug string
}

func (r Role) String() string {
	return r.slug
}

var (
	Unknown    = Role{""}
	Guest      = Role{"guest"}
	Student    = Role{"student"}
	Supervisor = Role{"supervisor"}
	Admin      = Role{"administrator"}
)

func FromString(s string) (*Role, error) {
	switch s {
	case Guest.slug:
		return &Guest, nil
	case Student.slug:
		return &Student, nil
	case Supervisor.slug:
		return &Supervisor, nil
	case Admin.slug:
		return &Admin, nil
	}

	return &Unknown, errors.New("unknown role: " + s)
}
