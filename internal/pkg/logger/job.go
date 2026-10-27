package logger

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
)

// WithJob marks ctx as one run of a background job. It mints a job_id, injects
// it together with the job name and component=job as log fields so every record
// of the run carries them, and returns the id for the caller's own records.
//
// A scheduler has no request to inherit a correlation id from, which is why its
// records used to be untraceable: one pass of a loop looked like any other.
func WithJob(ctx context.Context, name string) (context.Context, string) {
	id := newJobID()

	ctx = logging.InjectFields(ctx, logging.Fields{
		"job_id", id,
		"job", name,
		"component", "job",
	})
	return ctx, id
}

// newJobID mints a time-ordered id, so passes of one loop sort by wall clock.
func newJobID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// A missing id must not stop the job: a readable fallback keeps the
		// records joinable by name and time.
		return "job-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return id.String()
}
