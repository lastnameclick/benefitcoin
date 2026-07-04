package store

import "errors"

// ErrNotFound is returned by lookups when no row matches.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write can't proceed because of the row's
// current state (e.g. a bounty that's already claimed).
var ErrConflict = errors.New("conflict")

// ErrExpired is returned when a write targets a timeboxed row past its deadline.
var ErrExpired = errors.New("expired")
