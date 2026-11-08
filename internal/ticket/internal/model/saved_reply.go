package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// ReplyTitleMaxLen bounds a saved reply's title, in characters.
	ReplyTitleMaxLen = 80
	// MaxSavedReplies caps the staff's template list.
	MaxSavedReplies = 100
)

// ErrReplyTitleInvalid reports a title outside 1..ReplyTitleMaxLen.
var ErrReplyTitleInvalid = errors.New("title must be 1 to 80 characters")

// SavedReply is a staff reply template.
type SavedReply struct {
	Id        string
	Title     string
	Text      string
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewSavedReply validates and builds a template.
func NewSavedReply(title, text, by string, now time.Time) (*SavedReply, error) {
	r := &SavedReply{CreatedAt: now}
	if err := r.Edit(title, text, by, now); err != nil {
		return nil, err
	}
	return r, nil
}

// Edit replaces the title and text.
func (r *SavedReply) Edit(title, text, by string, now time.Time) error {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n == 0 || n > ReplyTitleMaxLen {
		return ErrReplyTitleInvalid
	}
	text, err := cleanText(text)
	if err != nil {
		return err
	}
	r.Title = title
	r.Text = text
	r.UpdatedBy = by
	r.UpdatedAt = now
	return nil
}

// Touch stamps the persisted update time (called by mongox.UpdateDoc).
func (r *SavedReply) Touch(now time.Time) { r.UpdatedAt = now }
