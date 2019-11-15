package storage

import "github.com/j18e/tempctl/models"

type Storage interface {
	// SomeonePresent reports whether at least one of the given users is
	// present.
	SomeonePresent([]*models.User) (bool, error)

	// CurrentTemp returns the current temperature in a named location.
	CurrentTemp(string) (float64, error)

	// WriteHeatingStatus writes the current status of a room's heater to the
	// storage.  0 indicates not heating, 1 indicates heating and -1 indicates
	// an error communicating with the heater.
	WriteHeatingStatus(string, int) error

	Close() error
}
