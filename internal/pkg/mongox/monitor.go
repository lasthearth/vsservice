package mongox

import (
	"container/list"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.uber.org/zap"
)

const (
	// componentMongo tags every driver record, so a log query separates
	// storage calls from gRPC, NATS and HTTP ones.
	componentMongo = "mongo"

	// eventCommandFailed and eventCommandSlow are the stable machine keys
	// dashboards and alerts filter on. The msg text may change, these must not.
	eventCommandFailed = "mongo.command.failed"
	eventCommandSlow   = "mongo.command.slow"

	// maxPendingCommands caps the correlation map. A command whose finished
	// event never arrives — a connection dropped in flight — would otherwise
	// leak one entry per command for the lifetime of the process.
	maxPendingCommands = 4096
)

// crudCommands are the commands whose document carries the target collection
// as the value of the field named after the command, for example
// {"find": "mods", ...}. The keys are lowercase because the server spells the
// cursor command "getMore" while the lookup is case-insensitive.
var crudCommands = map[string]struct{}{
	"aggregate": {},
	"count":     {},
	"delete":    {},
	"distinct":  {},
	"find":      {},
	"getmore":   {},
	"insert":    {},
	"update":    {},
}

// commandMonitor turns driver command events into log records. It never logs
// the command document: filters, updates and credentials travel inside it. The
// command name and the collection are the only parts of a document it reads.
type commandMonitor struct {
	log    logger.Logger
	slowMS int64

	mu sync.Mutex
	// pending maps a request id to its in-flight command, because the
	// collection exists only on the started event and the finished events do
	// not repeat it. order holds the same entries in arrival order so evicting
	// the oldest one stays constant time; a linked list keeps removing a
	// finished command from the middle cheap as well.
	pending map[int64]*list.Element
	order   *list.List
}

// inFlight is what a finished event needs from the started event it belongs to.
type inFlight struct {
	requestID  int64
	collection string
}

// newCommandMonitor builds the correlation state behind the driver callbacks.
// slowMS at or below zero switches slow-command logging off.
func newCommandMonitor(log logger.Logger, slowMS int64) *commandMonitor {
	return &commandMonitor{
		log:     log.WithComponent(componentMongo),
		slowMS:  slowMS,
		pending: make(map[int64]*list.Element),
		order:   list.New(),
	}
}

// monitor returns the callbacks the driver calls. The driver runs them on its
// own goroutines, so every method is safe for concurrent use.
func (m *commandMonitor) monitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started:   m.started,
		Succeeded: m.succeeded,
		Failed:    m.failed,
	}
}

// started records the collection of an in-flight command. It logs nothing:
// successful fast commands are the overwhelming majority and would drown the
// records that matter.
func (m *commandMonitor) started(_ context.Context, e *event.CommandStartedEvent) {
	if e == nil {
		return
	}
	m.track(e.RequestID, collectionOf(e.CommandName, e.Command))
}

// succeeded reports a command that took at least slowMS milliseconds.
func (m *commandMonitor) succeeded(ctx context.Context, e *event.CommandSucceededEvent) {
	if e == nil {
		return
	}
	collection := m.finish(e.RequestID)

	// A non-positive threshold disables slow logging, so nothing is logged on
	// success in that mode.
	if m.slowMS <= 0 || e.Duration < time.Duration(m.slowMS)*time.Millisecond {
		return
	}

	logger.From(ctx, m.log).Warn("mongo command slow",
		commandFields(eventCommandSlow, e.CommandName, e.DatabaseName, collection, e.Duration)...)
}

// failed reports a command the driver could not complete. Every failure is
// logged: it is either a bug or an outage, and both need the record.
func (m *commandMonitor) failed(ctx context.Context, e *event.CommandFailedEvent) {
	if e == nil {
		return
	}
	collection := m.finish(e.RequestID)

	fields := commandFields(eventCommandFailed, e.CommandName, e.DatabaseName, collection, e.Duration)
	// zap.Error emits nothing for a nil Failure, so the record stays valid if
	// the driver ever reports a failure without a cause.
	fields = append(fields, zap.Error(e.Failure))

	logger.From(ctx, m.log).Error("mongo command failed", fields...)
}

// commandFields builds the shared part of both record kinds.
func commandFields(eventName, commandName, databaseName, collection string, d time.Duration) []zap.Field {
	fields := []zap.Field{
		zap.String("event", eventName),
		zap.String("command_name", commandName),
		zap.String("database_name", databaseName),
	}
	// An empty collection would read as a real collection name in a query, so
	// the field is omitted instead of left blank.
	if collection != "" {
		fields = append(fields, zap.String("collection_name", collection))
	}
	return append(fields, zap.Int64("duration_ms", d.Milliseconds()))
}

// track stores an in-flight command and evicts the oldest entries past the
// cap. A repeated request id replaces the previous entry, because leaving it
// behind would keep a dead command in the map until eviction reached it.
func (m *commandMonitor) track(requestID int64, collection string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if el, ok := m.pending[requestID]; ok {
		m.order.Remove(el)
	}
	m.pending[requestID] = m.order.PushBack(inFlight{requestID: requestID, collection: collection})

	for m.order.Len() > maxPendingCommands {
		// Front is non-nil whenever Len is positive, so the cast cannot panic.
		oldest := m.order.Front()
		m.order.Remove(oldest)
		delete(m.pending, oldest.Value.(inFlight).requestID)
	}
}

// finish drops an in-flight entry and returns the collection it carried. It
// holds the lock for the map only: logging happens in the caller, so a slow
// writer cannot stall the driver.
func (m *commandMonitor) finish(requestID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	el, ok := m.pending[requestID]
	if !ok {
		// No started event for this id: either the cap evicted it or the
		// driver reported a finish alone. Both leave nothing to correlate.
		return ""
	}
	delete(m.pending, requestID)
	m.order.Remove(el)
	return el.Value.(inFlight).collection
}

// collectionOf reads the collection name out of a command document. Only the
// CRUD commands carry it, as the value of their first field; every other
// command returns "" and the caller omits the field. Nothing else in the
// document is read, because it holds user content.
func collectionOf(commandName string, cmd bson.Raw) string {
	if _, ok := crudCommands[strings.ToLower(commandName)]; !ok {
		return ""
	}

	elements, err := cmd.Elements()
	if err != nil {
		// The driver handed over bytes that are not a well-formed document.
		// The record is still useful without the collection, so this is not
		// worth failing the callback over.
		return ""
	}
	if len(elements) == 0 {
		return ""
	}

	first := elements[0]
	if !strings.EqualFold(first.Key(), commandName) {
		return ""
	}
	// A database-level command such as {"aggregate": 1} has no collection.
	collection, ok := first.Value().StringValueOK()
	if !ok {
		return ""
	}
	return collection
}
