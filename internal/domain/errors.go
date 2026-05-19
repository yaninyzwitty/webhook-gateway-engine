package domain

import "errors"

var (
	ErrDuplicateEvent   = errors.New("duplicate event")
	ErrEndpointNotFound = errors.New("endpoint not found")
)