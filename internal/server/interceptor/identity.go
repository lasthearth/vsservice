package interceptor

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	middleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// RequestIDMetadataKey is the metadata and HTTP header name that carries the
// correlation id in both directions. The gateway forwards it, identity adopts
// it, and the response echoes it back so a support ticket names the exact call.
const RequestIDMetadataKey = "x-request-id"

// maxIncomingRequestID rejects an absurd client-supplied id before it reaches
// a log line or a response header.
const maxIncomingRequestID = 64

// Identity assigns a request_id to every call, including the public methods
// the auth interceptor never sees, and returns it to the caller. It runs
// first, after recovery, so every record of one user action shares one id.
type Identity struct{}

func NewIdentity() *Identity { return &Identity{} }

func (*Identity) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, rid := withRequestID(ctx, info.FullMethod)
		// A failure to set the header must not fail the call: the id is still
		// in the logs, the client just cannot quote it.
		_ = grpc.SetHeader(ctx, metadata.Pairs(RequestIDMetadataKey, rid))
		return handler(ctx, req)
	}
}

func (*Identity) Stream() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, rid := withRequestID(stream.Context(), info.FullMethod)
		// A header failure must not fail the stream: the record keeps the id.
		_ = stream.SetHeader(metadata.Pairs(RequestIDMetadataKey, rid))

		wrapped := middleware.WrapServerStream(stream)
		wrapped.WrappedContext = ctx
		return handler(srv, wrapped)
	}
}

// withRequestID adopts a valid incoming id or mints one, then publishes it as
// a middleware log field and as the value GetRequestID reads.
func withRequestID(ctx context.Context, method string) (context.Context, string) {
	if rid, err := GetRequestID(ctx); err == nil && rid != "" {
		return ctx, rid
	}

	rid := incomingRequestID(ctx)
	if rid == "" {
		rid = newRequestID()
	}

	ctx = logging.InjectFields(ctx, logging.Fields{
		"request_id", rid,
		"method", method,
	})
	return context.WithValue(ctx, ctxKey{"rid"}, rid), rid
}

// incomingRequestID returns the caller-supplied id when it parses as a uuid,
// so a gateway, proxy or client can keep one id across the whole chain.
func incomingRequestID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(RequestIDMetadataKey)
	if len(values) == 0 {
		return ""
	}
	rid := strings.TrimSpace(values[0])
	if rid == "" || len(rid) > maxIncomingRequestID {
		return ""
	}
	if _, err := uuid.Parse(rid); err != nil {
		return ""
	}
	return rid
}

// newRequestID mints a time-ordered id. uuid v7 keeps log order close to
// wall-clock order, which is what a replay query needs. A generator failure is
// not a reason to refuse the call: a readable fallback id beats an error.
func newRequestID() string {
	rid, err := uuid.NewV7()
	if err != nil {
		return "noid-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return rid.String()
}
