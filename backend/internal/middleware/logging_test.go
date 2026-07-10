package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// TestRequestLoggerCorrelation is the load-bearing guarantee of the logging
// setup: a field a downstream handler adds via UpdateContext (as the auth
// middleware does with the resolved user) must appear on BOTH that handler's
// own lines and the single access-log line the outer RequestLogger emits, and
// every line must share the same request_id.
func TestRequestLoggerCorrelation(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf) // no timestamp: keeps assertions simple

	// Downstream handler mimics auth: enrich the request logger in place, then
	// emit its own line, then respond 403.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zerolog.Ctx(r.Context()).UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("user", "alice@example.com")
		})
		zerolog.Ctx(r.Context()).Warn().Msg("handler line")
		w.WriteHeader(http.StatusForbidden)
	})

	// chi RequestID -> RequestLogger -> inner
	h := chimiddleware.RequestID(RequestLogger(base)(inner))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/purchase-requests/7", nil))

	// Parse each JSON log line.
	var lines []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("log line is not JSON: %s (%v)", raw, err)
		}
		lines = append(lines, m)
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines (handler + access), got %d: %s", len(lines), buf.String())
	}

	handlerLine, accessLine := lines[0], lines[1]

	// request_id present and identical on both lines.
	rid, _ := handlerLine["request_id"].(string)
	if rid == "" {
		t.Fatalf("handler line missing request_id: %v", handlerLine)
	}
	if accessLine["request_id"] != rid {
		t.Fatalf("request_id mismatch: handler=%q access=%v", rid, accessLine["request_id"])
	}

	// The user added downstream must appear on the access line too (in-place
	// enrichment is visible to the outer middleware).
	if handlerLine["user"] != "alice@example.com" {
		t.Errorf("handler line missing user: %v", handlerLine)
	}
	if accessLine["user"] != "alice@example.com" {
		t.Errorf("access line missing user (UpdateContext did not propagate): %v", accessLine)
	}

	// Access line carries method/path/status and is the "request" message.
	if accessLine["message"] != "request" {
		t.Errorf("access message = %v, want \"request\"", accessLine["message"])
	}
	if accessLine["method"] != http.MethodGet || accessLine["path"] != "/purchase-requests/7" {
		t.Errorf("access line method/path wrong: %v", accessLine)
	}
	if accessLine["status"] != float64(http.StatusForbidden) {
		t.Errorf("access status = %v, want 403", accessLine["status"])
	}
}

// TestRequestLoggerIsolatesRequests guards against a shared-logger bug: fields
// added while serving one request must not leak into another's access line.
func TestRequestLoggerIsolatesRequests(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf)

	makeHandler := func(user string) http.Handler {
		return chimiddleware.RequestID(RequestLogger(base)(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				zerolog.Ctx(r.Context()).UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str("user", user)
				})
				w.WriteHeader(http.StatusOK)
			})))
	}

	rec := httptest.NewRecorder()
	makeHandler("alice").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/a", nil))
	makeHandler("bob").ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/b", nil))

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("want 2 access lines, got %d", len(lines))
	}
	var a, b map[string]any
	_ = json.Unmarshal(lines[0], &a)
	_ = json.Unmarshal(lines[1], &b)
	if a["user"] != "alice" || b["user"] != "bob" {
		t.Fatalf("request logger leaked state across requests: a=%v b=%v", a["user"], b["user"])
	}
}
