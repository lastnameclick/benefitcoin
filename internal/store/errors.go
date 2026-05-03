package store

import "errors"

// ErrNotFound is returned by lookups when no row matches.
var ErrNotFound = errors.New("not found")
