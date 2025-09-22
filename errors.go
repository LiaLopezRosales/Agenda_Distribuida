// errors.go
package agendadistribuida

import "errors"

// ErrNotImplemented is returned by placeholder functions that will be implemented later.
var ErrNotImplemented = errors.New("not implemented")

// ErrUnauthorized is returned when the current subject lacks permissions.
var ErrUnauthorized = errors.New("unauthorized")

// ErrInvalidInput is returned when the input fails validation.
var ErrInvalidInput = errors.New("invalid input")
