package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

// reqLog returns the request-scoped logger installed by
// middleware.RequestLogger — it carries the request_id and (after auth) the
// authenticated user, so every line a handler emits can be correlated with the
// access-log line. Falls back to a disabled logger if the middleware did not
// run (e.g. a unit test that invokes a handler without the router), so callers
// never need a nil check.
func reqLog(r *http.Request) *zerolog.Logger {
	return zerolog.Ctx(r.Context())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}

func urlParam(r *http.Request, param string) string {
	return chi.URLParam(r, param)
}

// parseInt64Param parses an optional int64 query-string value. It returns nil
// when the value is empty or not a valid integer, so callers can treat a bad or
// absent value as "no filter" rather than an error.
func parseInt64Param(s string) *int64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}
