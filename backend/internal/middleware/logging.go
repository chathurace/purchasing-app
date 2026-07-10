package middleware

import (
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// RequestLogger attaches a request-scoped zerolog.Logger (carrying the chi
// request_id) to the request context and emits exactly one access-log line per
// request with method, path, status, response size and latency. Handlers read
// the logger with zerolog.Ctx(r.Context()) (see handler.reqLog); the auth
// middleware enriches it in place with the resolved user, so every downstream
// error line and the access line share the same request_id/user fields and can
// be correlated.
//
// Ordering: install AFTER chimiddleware.RequestID (so the id exists) and, to
// still log requests whose handler panics, OUTSIDE chimiddleware.Recoverer —
// Recoverer converts the panic into a 500 and returns normally, which this
// middleware then records.
func RequestLogger(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Fresh child logger per request (a new *Logger, so concurrent
			// requests never share state). zerolog.Ctx on any descendant
			// context returns this same pointer, which auth mutates in place.
			l := base.With().
				Str("request_id", chimiddleware.GetReqID(r.Context())).
				Logger()
			ctx := l.WithContext(r.Context())
			r = r.WithContext(ctx)

			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK // handler wrote body without an explicit WriteHeader
			}
			lg := zerolog.Ctx(ctx)
			var evt *zerolog.Event
			switch {
			case status >= 500:
				evt = lg.Error()
			case status >= 400:
				evt = lg.Warn()
			default:
				evt = lg.Info()
			}
			evt.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", status).
				Int("bytes", ww.BytesWritten()).
				Dur("duration", time.Since(start)).
				Msg("request")
		})
	}
}
