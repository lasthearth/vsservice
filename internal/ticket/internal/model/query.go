package model

// Viewer tells whose list is read: a player sees their own tickets ordered by
// the last message they can see, the staff every ticket ordered by the last
// message of all.
type Viewer int

const (
	ViewerAuthor Viewer = iota
	ViewerStaff
)

// AssigneeScope narrows the staff inbox by who took a ticket.
type AssigneeScope int

const (
	AssigneeAny AssigneeScope = iota
	// AssigneeMine is the tickets StaffId took.
	AssigneeMine
	// AssigneeNobody is the tickets nobody took.
	AssigneeNobody
)

// Query is one page request of a ticket list. Zero values mean "no filter".
type Query struct {
	Viewer   Viewer
	AuthorId string
	Status   Status
	Category Category
	Assignee AssigneeScope
	// StaffId is the caller, for AssigneeMine.
	StaffId   string
	Limit     int
	PageToken string
}

// StaffCounters are the staff inbox badges.
type StaffCounters struct {
	Open           int64
	OpenUnassigned int64
	OpenMine       int64
	Unread         int64
}
