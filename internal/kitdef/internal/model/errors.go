package model

import "errors"

// ErrEmptyTitle is returned by Rename when the supplied title is empty.
var ErrEmptyTitle = errors.New("kit title cannot be empty")
