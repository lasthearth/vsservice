package logger

import (
	"context"

	"go.uber.org/zap"
)

// Op builds one domain-operation record: who did what to which entity and
// what changed. Services emit it next to the model method that mutates state,
// so the log states the transition instead of the storage call.
type Op struct {
	log    Logger
	fields []zap.Field
}

// OpFrom starts a record carrying the ctx correlation fields (request_id,
// user_id) and component=op.
func OpFrom(ctx context.Context, fallback Logger) *Op {
	return &Op{
		log:    From(ctx, fallback),
		fields: []zap.Field{zap.String("component", "op")},
	}
}

// Event sets the stable machine key dashboards filter on, e.g.
// "settlement.request.approved".
func (o *Op) Event(name string) *Op {
	return o.add(zap.String("event", name))
}

// Entity names the affected record.
func (o *Op) Entity(kind, id string) *Op {
	return o.add(zap.String("entity", kind), zap.String("entity_id", id))
}

// Actor names who caused the change.
func (o *Op) Actor(id string) *Op {
	return o.add(zap.String("actor_id", id))
}

// Transition records a state change.
func (o *Op) Transition(from, to string) *Op {
	return o.add(zap.String("from_state", from), zap.String("to_state", to))
}

// Reason records why the transition happened, including refusals.
func (o *Op) Reason(reason string) *Op {
	return o.add(zap.String("reason", reason))
}

// Field adds arbitrary extra fields.
func (o *Op) Field(fields ...zap.Field) *Op {
	return o.add(fields...)
}

func (o *Op) add(fields ...zap.Field) *Op {
	o.fields = append(o.fields, fields...)
	return o
}

// Info emits the record at Info.
func (o *Op) Info(msg string) {
	o.log.Info(msg, o.fields...)
}

// Warn emits the record at Warn, for a refused or partial operation.
func (o *Op) Warn(msg string) {
	o.log.Warn(msg, o.fields...)
}

// Error emits the record at Error with the cause attached.
func (o *Op) Error(msg string, err error) {
	o.log.Error(msg, append(o.fields, zap.Error(err))...)
}
