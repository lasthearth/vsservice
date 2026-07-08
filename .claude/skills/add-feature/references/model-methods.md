# Mutate models through their own methods

Reference for SKILL.md Step 4. Domain models are the only place business rules live. External code (services, repositories) changes model state by **calling methods** on the model, never by setting fields. The custom linter enforces this.

Methods that can fail validation return `error`; pure setters that always succeed don't.

## Validation + normalization — `internal/settlement/model/settlement.go`

```go
type Settlement struct {
	Id            string
	Name          string
	Type          SettlementType
	Description   string
	Diplomacy     string
	ImperialFavor int64
	// …
}

func (s *Settlement) SetDiplomacy(diplomacy string) error {
	if diplomacy == "" {
		return errors.New("diplomacy cannot be empty")
	}
	r, size := utf8.DecodeRuneInString(diplomacy)
	s.Diplomacy = string(unicode.ToUpper(r)) + diplomacy[size:]
	return nil
}

func (s *Settlement) SetProfile(name, description string, attachments []Attachment) {
	s.Name = name
	s.Description = description
	s.Attachments = attachments
}

func (s *Settlement) AddFavor(amount int64)              { s.ImperialFavor += amount }
func (s *Settlement) DeductFavor(amount int64) error {
	if s.ImperialFavor < amount {
		return errors.New("insufficient imperial favor")
	}
	s.ImperialFavor -= amount
	return nil
}
```

## State machine — `internal/player/internal/model/verification/verification.go`

State transitions are methods that guard against illegal moves:

```go
func (v *Verification) Approve() error {
	if v.Status != VerificationStatusPending {
		return ErrInvalidTransition
	}
	v.Status = VerificationStatusApproved
	v.UpdatedAt = time.Now()
	return nil
}

func (v *Verification) Reject(reason string) error {
	if v.Status != VerificationStatusPending {
		return ErrInvalidTransition
	}
	v.Status = VerificationStatusRejected
	v.RejectionReason = reason
	v.UpdatedAt = time.Now()
	return nil
}
```

## Timestamped transition — `internal/kit/internal/model/assignment.go`

```go
func (ka *KitAssignment) TransitionTo(status AssignmentStatus) error {
	if !status.IsValid() {
		return errors.New("invalid assignment status")
	}
	if !ka.isValidTransition(status) {
		return errors.New("invalid status transition")
	}
	switch status {
	case AssignmentStatusDelivered:
		now := time.Now()
		ka.DeliveredAt = &now
	case AssignmentStatusClaimed:
		now := time.Now()
		ka.ClaimedAt = &now
	}
	ka.Status = status
	return nil
}
```

## How this connects to the rest

- The **service** calls these methods — usually inside a repository callback (see `repository-update.md`).
- The **model struct has no bson tags**; it is mapped to/from the persistence dto by goverter (see `goverter.md`). Mongo shape stays in `dto/mongo/`, domain shape stays in `model/`.
- When adding a new mutable field, add the field to the model **and** a method that sets it (with whatever validation/normalization the rule requires). Do both in the same change.
