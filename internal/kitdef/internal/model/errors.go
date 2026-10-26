package model

import "errors"

// ErrEmptyTitle is returned by Rename when the supplied title is empty.
var ErrEmptyTitle = errors.New("kit title cannot be empty")

// ErrUnknownCategory is returned by SetCategory when the supplied category is
// not one of the three KitCategory* constants.
var ErrUnknownCategory = errors.New("unknown kit category")

// ErrItemIndex is returned by RemoveItemAt when the index is outside the
// kit's items.
var ErrItemIndex = errors.New("kit item index out of range")

// ErrLastItem is returned by RemoveItemAt when the removal would leave the
// kit without items: an empty kit is treated as missing downstream.
var ErrLastItem = errors.New("cannot remove the last item of a kit")
