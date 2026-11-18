package serverinfo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	// streamPollEvery is how often the poller reads the document. The game
	// server writes it every 5 seconds, so reading faster would only return the
	// same numbers again.
	streamPollEvery = 5 * time.Second

	// streamPollTimeout bounds one read. Without it a hung Mongo call stops the
	// poller for as long as the driver keeps retrying, and every subscriber
	// silently keeps the numbers of the last good read.
	streamPollTimeout = 4 * time.Second

	// streamKeepAlive is how long a connection may stay silent before a comment
	// line is written. A proxy or a browser drops an idle response long before
	// an unchanged world clock changes again, and 25 seconds is inside every
	// proxy default this service sits behind.
	streamKeepAlive = 25 * time.Second

	// streamRetryMillis is the reconnect delay the browser is told to use. It is
	// sent as the SSE "retry" field before the first event.
	streamRetryMillis = 5000

	// streamMaxSubscribers caps concurrent connections. Every subscriber is a
	// goroutine and a socket on one process, and the feed is public: without a
	// cap one scanner that never closes a connection takes the gateway down
	// with it.
	streamMaxSubscribers = 1000

	// streamQueue is how many unpublished frames one subscriber may fall behind.
	// The poller publishes at most one frame per poll, so eight is forty seconds
	// of backlog; past it the client is not reading and is dropped rather than
	// buffered without a bound.
	streamQueue = 8

	// streamWriteTimeout bounds one write. A client that stops reading fills the
	// socket buffer and then blocks the handler in Write; the deadline turns
	// that into an error, and the subscriber goes away.
	streamWriteTimeout = 10 * time.Second

	// streamSubscribeTimeout bounds the read a subscriber does when it arrives
	// before the poller's first pass. It only matters in the first moments of a
	// process, where waiting for the poller would mean no first frame.
	streamSubscribeTimeout = 5 * time.Second

	// streamRetryAfterSeconds is the Retry-After value of a 503: ten seconds, so
	// a rejected client does not hammer a listener that is already at its cap.
	streamRetryAfterSeconds = 10

	// streamEvent is the SSE event name, and streamEventJob names the poller in
	// its records.
	streamEvent    = "serverinfo"
	streamEventJob = "serverinfo.stream"

	// The event keys of the records this file writes. Each one is a stable
	// dashboard key, not a message.
	streamPollFailed  = "serverinfo.stream.poll.failed"
	streamPollOK      = "serverinfo.stream.poll.ok"
	streamDropped     = "serverinfo.stream.dropped"
	streamRejected    = "serverinfo.stream.rejected"
	streamUnavailable = "serverinfo.stream.unavailable"
)

var (
	// errStreamFull is a listener already at streamMaxSubscribers.
	errStreamFull = errors.New("the serverinfo stream is at its subscriber limit")

	// retryFrame is the first thing a subscriber reads: the reconnect delay.
	// EventSource only applies the field when it arrives on its own line, so it
	// is a frame like any other.
	retryFrame = []byte("retry: " + strconv.Itoa(streamRetryMillis) + "\n\n")

	// keepAliveFrame is an SSE comment: the client ignores it, the connection
	// does not go idle.
	keepAliveFrame = []byte(": ping\n\n")
)

// liveInfo is one frame's payload. The field names are the frozen contract the
// landing site parses: online, max_online and time, the same names
// GET /v1/serverinfo/totalonline and GET /v1/serverinfo/worldtime emit under
// protojson's UseProtoNames.
type liveInfo struct {
	Online    int32  `json:"online"`
	MaxOnline int32  `json:"max_online"`
	Time      string `json:"time"`
}

// frame renders the payload as one SSE event. The blank line ends the event:
// without it the browser holds the data until the next frame arrives.
func (i liveInfo) frame() []byte {
	// The payload is a struct of int32 and string, so marshal cannot fail. The
	// error is not discarded on the floor: a nil frame keeps a half-built event
	// from ever reaching a client.
	data, err := json.Marshal(i)
	if err != nil {
		return nil
	}

	frame := make([]byte, 0, len(streamEvent)+len(data)+16)
	frame = append(frame, "event: "...)
	frame = append(frame, streamEvent...)
	frame = append(frame, "\ndata: "...)
	frame = append(frame, data...)
	frame = append(frame, '\n', '\n')
	return frame
}

// Stream serves GET /v1/serverinfo/stream: the online count and the world clock
// as a server-sent event feed.
//
// One poller reads the singleton document for the whole process and publishes
// only on a change, so N subscribers cost one Mongo read per five seconds. The
// route is plain HTTP because grpc-gateway v2 cannot frame an event stream.
type Stream struct {
	repo Repository
	log  logger.Logger

	// The tunables are fields so a test can shrink them; nothing outside this
	// package sets them.
	pollEvery    time.Duration
	pollTimeout  time.Duration
	keepAlive    time.Duration
	writeTimeout time.Duration
	readTimeout  time.Duration
	maxSubs      int
	queue        int

	mu sync.Mutex
	// subs holds one channel per live connection. A channel is closed by
	// whoever removes the subscriber, exactly once.
	subs map[chan []byte]struct{}
	// now is the last published snapshot; known says there is one. A subscriber
	// that arrives before the first read takes its frame from the document
	// itself, and the poller's first publish then also corrects it.
	now   liveInfo
	known bool
	// failing remembers a run of failed reads, so a Mongo outage writes one
	// record instead of one every five seconds.
	failing bool
}

// NewStream builds the feed. The poller starts with Run, which the fx lifecycle
// hook calls.
func NewStream(log logger.Logger, repo Repository) *Stream {
	return &Stream{
		repo:         repo,
		log:          log.WithComponent("serverinfo-stream"),
		pollEvery:    streamPollEvery,
		pollTimeout:  streamPollTimeout,
		keepAlive:    streamKeepAlive,
		writeTimeout: streamWriteTimeout,
		readTimeout:  streamSubscribeTimeout,
		maxSubs:      streamMaxSubscribers,
		queue:        streamQueue,
		subs:         make(map[chan []byte]struct{}),
	}
}

// Run polls the document until ctx is done, publishing one frame per change.
//
// The loop's lifetime is the app's: the fx hook starts it in a goroutine on a
// context of its own, because the hook context is cancelled about 15 seconds
// after start.
func (s *Stream) Run(ctx context.Context) {
	// One job id for the whole loop: its records have no request to inherit a
	// correlation id from, and a per-read id would make one loop look like a
	// thousand jobs.
	ctx, _ = logger.WithJob(ctx, streamEventJob)

	s.poll(ctx, logger.From(ctx, s.log).WithMethod("poll"))

	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(ctx, logger.From(ctx, s.log).WithMethod("poll"))
		}
	}
}

// poll reads the document once and publishes it when it differs from the last
// frame. An error keeps the last snapshot: a feed that says nothing is better
// than a feed that says zero, and the next tick retries.
func (s *Stream) poll(ctx context.Context, log logger.Logger) {
	ctx, cancel := context.WithTimeout(ctx, s.pollTimeout)
	defer cancel()

	info, err := s.read(ctx)
	if err != nil {
		s.mu.Lock()
		first := !s.failing
		s.failing = true
		subscribers := len(s.subs)
		s.mu.Unlock()

		if first {
			// One record per outage: at five seconds a pass, an unguarded Warn
			// is twelve lines a minute per instance saying the same thing.
			log.Warn("failed to read serverinfo, keeping the last frame",
				zap.String("event", streamPollFailed),
				zap.Int("subscribers", subscribers),
				zap.Error(err))
		}
		return
	}

	s.mu.Lock()
	recovered := s.failing
	s.failing = false
	// An unknown snapshot counts as a change: a subscriber that joined before
	// this read took its first frame from its own read, and this frame either
	// agrees with it or corrects it.
	changed := !s.known || s.now != info
	s.mu.Unlock()

	if recovered {
		log.Info("serverinfo reads recovered", zap.String("event", streamPollOK))
	}

	if changed {
		s.publish(info)
	}
}

// read builds the frame payload from the document.
func (s *Stream) read(ctx context.Context) (liveInfo, error) {
	doc, err := s.repo.GetServerInfo(ctx)
	if err != nil {
		return liveInfo{}, err
	}

	return liveInfo{
		Online:    int32(doc.TotalOnline),
		MaxOnline: int32(doc.MaxOnline),
		Time:      doc.WorldTime,
	}, nil
}

// publish stores the snapshot and hands one frame to every subscriber.
func (s *Stream) publish(info liveInfo) {
	frame := info.frame()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.now = info
	s.known = true

	if frame == nil {
		return
	}

	for ch := range s.subs {
		select {
		case ch <- frame:
		default:
			// The queue is full: this client reads slower than the feed
			// publishes. Dropping it is the only bounded answer — waiting on it
			// would hold the poller, and growing the queue would hold memory
			// for a reader that may never come back.
			delete(s.subs, ch)
			close(ch)
		}
	}
}

// subscribe registers a connection and returns the channel its frames arrive
// on, already holding the first one. The channel is bidirectional so the caller
// can hand it back to unsubscribe; it only reads from it.
func (s *Stream) subscribe(ctx context.Context) (chan []byte, error) {
	ch := make(chan []byte, s.queue)

	s.mu.Lock()
	if len(s.subs) >= s.maxSubs {
		s.mu.Unlock()
		return nil, errStreamFull
	}
	if s.known {
		// The common path: the poller has a snapshot, so the first frame is the
		// current state and costs no database read.
		if frame := s.now.frame(); frame != nil {
			ch <- frame
		}
		s.subs[ch] = struct{}{}
		s.mu.Unlock()
		return ch, nil
	}
	s.mu.Unlock()

	// The poller has not finished its first read yet. The first frame must not
	// wait for it, so this subscriber reads the document itself. The window
	// between this read and the registration below is microseconds into the
	// life of the process, and the poller's first publish covers it.
	info, err := s.readWithTimeout(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if len(s.subs) >= s.maxSubs {
		s.mu.Unlock()
		return nil, errStreamFull
	}
	if frame := info.frame(); frame != nil {
		ch <- frame
	}
	s.subs[ch] = struct{}{}
	s.mu.Unlock()

	return ch, nil
}

// readWithTimeout is the cold-start read: a bounded one, because the request
// must answer rather than hold the connection while the driver retries.
func (s *Stream) readWithTimeout(ctx context.Context) (liveInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()

	return s.read(ctx)
}

// unsubscribe removes a subscriber and closes its channel. It is idempotent:
// publish closes and forgets a subscriber whose queue overflowed, and the
// handler still calls this on the way out.
func (s *Stream) unsubscribe(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.subs[ch]; !ok {
		return
	}
	delete(s.subs, ch)
	close(ch)
}

// ServeHTTP serves one subscription: the headers, the retry frame, then a frame
// per change and a comment per streamKeepAlive until the client leaves.
func (s *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.From(ctx, s.log).WithMethod("serve")

	ch, err := s.subscribe(ctx)
	if err != nil {
		// Both refusals are temporary: the cap is a moment of load, and a
		// failed read is a database that is down. Ten seconds is long enough for
		// the client not to hammer and short enough for the widget to recover.
		event := streamUnavailable
		if errors.Is(err, errStreamFull) {
			event = streamRejected
		}
		log.Warn("refused a serverinfo subscriber",
			zap.String("event", event),
			zap.Error(err))

		w.Header().Set("Retry-After", strconv.Itoa(streamRetryAfterSeconds))
		http.Error(w, "the serverinfo stream is unavailable", http.StatusServiceUnavailable)
		return
	}
	defer s.unsubscribe(ch)

	rc := http.NewResponseController(w)

	// Headers must be set before the first write; after WriteHeader the response
	// carries whatever was in the map at that moment.
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	// A cached event stream is a frozen widget: the connection is the state.
	w.Header().Set("Cache-Control", "no-store")
	// nginx buffers a proxied response unless told otherwise, and a buffered
	// event stream arrives as one lump when the connection ends.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// The write deadline below belongs to this response only. Clearing it keeps
	// a reused keep-alive connection from failing the next write on a deadline
	// that has already passed.
	defer func() {
		_ = rc.SetWriteDeadline(time.Time{})
	}()

	if !writeFrame(rc, w, retryFrame, s.writeTimeout) {
		return
	}

	timer := time.NewTimer(s.keepAlive)
	defer timer.Stop()

	reset := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(s.keepAlive)
	}

	for {
		select {
		case <-ctx.Done():
			// The client closed the connection or the process is stopping. The
			// deferred unsubscribe is what frees the slot and the poller's
			// reference to this channel.
			return
		case frame, ok := <-ch:
			if !ok {
				log.Warn("dropping a slow serverinfo subscriber",
					zap.String("event", streamDropped))
				return
			}
			if !writeFrame(rc, w, frame, s.writeTimeout) {
				return
			}
			reset()
		case <-timer.C:
			if !writeFrame(rc, w, keepAliveFrame, s.writeTimeout) {
				return
			}
			reset()
		}
	}
}

// writeFrame writes one SSE block and flushes it. False means the client is
// gone or stopped reading, and the caller returns.
func writeFrame(rc *http.ResponseController, w http.ResponseWriter, frame []byte, timeout time.Duration) bool {
	// A client that stops reading fills the socket buffer and then blocks this
	// goroutine in Write for as long as the kernel keeps the connection. The
	// deadline turns that wait into an error.
	_ = rc.SetWriteDeadline(time.Now().Add(timeout))

	if _, err := w.Write(frame); err != nil {
		return false
	}

	return rc.Flush() == nil
}
