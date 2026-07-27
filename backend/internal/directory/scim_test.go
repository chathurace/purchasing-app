package directory

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestListAllPagination drives listAll against a fake SCIM server that paginates
// and returns both email shapes (bare string and object), verifying the client
// walks every page and maps resources correctly.
func TestListAllPagination(t *testing.T) {
	// Three users across two pages (pageSize 2). Page 1 uses object emails, page 2
	// a bare-string email; the third user has no name (derived from email).
	pages := map[int]string{
		1: `{"totalResults":3,"startIndex":1,"itemsPerPage":2,"Resources":[
			{"userName":"PRIMARY/alice","name":{"givenName":"Alice","familyName":"Ng"},"emails":[{"value":"alice@wso2.com","primary":true}]},
			{"userName":"bob","name":{"givenName":"Bob","familyName":"Roy"},"emails":[{"value":"bob-alt@wso2.com"},{"value":"bob@wso2.com","primary":true}]}
		]}`,
		3: `{"totalResults":3,"startIndex":3,"itemsPerPage":2,"Resources":[
			{"userName":"carol@wso2.com","name":{"givenName":"","familyName":""},"emails":["carol@wso2.com"]}
		]}`,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("count"); got != "2" {
			t.Errorf("count = %q, want 2", got)
		}
		startIndex := r.URL.Query().Get("startIndex")
		body, ok := pages[atoi(startIndex)]
		if !ok {
			body = `{"totalResults":3,"startIndex":0,"itemsPerPage":0,"Resources":[]}`
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := &scimClient{usersURL: srv.URL + "/Users", pageSize: 2, http: srv.Client()}
	users, err := c.listAll(context.Background())
	if err != nil {
		t.Fatalf("listAll: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("got %d users, want 3: %+v", len(users), users)
	}

	want := map[string]string{
		"alice@wso2.com": "Alice Ng",
		"bob@wso2.com":   "Bob Roy", // primary chosen over the earlier bob-alt
		"carol@wso2.com": "carol",   // no name → local part of email
	}
	for _, u := range users {
		w, ok := want[u.Email]
		if !ok {
			t.Errorf("unexpected email %q", u.Email)
			continue
		}
		if u.Name != w {
			t.Errorf("email %q: name = %q, want %q", u.Email, u.Name, w)
		}
	}
}

// TestServiceCacheAndForcedRefresh verifies the cache serves within its TTL and
// that a forced refresh arriving right after a fetch is coalesced onto the cache
// (rate-limited) rather than hitting SCIM again.
func TestServiceCacheAndForcedRefresh(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body := `{"totalResults":0,"startIndex":0,"itemsPerPage":0,"Resources":[]}`
		if atoi(r.URL.Query().Get("startIndex")) == 1 {
			body = `{"totalResults":1,"startIndex":1,"itemsPerPage":100,"Resources":[
				{"userName":"gina","name":{"givenName":"Gina","familyName":"Lee"},"emails":[{"value":"gina@wso2.com","primary":true}]}
			]}`
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	s := &Service{
		client: &scimClient{usersURL: srv.URL + "/Users", pageSize: 100, http: srv.Client()},
		ttl:    time.Minute,
		log:    zerolog.Nop(),
	}
	ctx := context.Background()

	if _, err := s.List(ctx, false); err != nil {
		t.Fatalf("first List: %v", err)
	}
	if _, err := s.List(ctx, false); err != nil { // served from cache
		t.Fatalf("second List: %v", err)
	}
	if _, err := s.List(ctx, true); err != nil { // forced, but recent → coalesced
		t.Fatalf("forced List: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("SCIM fetched %d times, want 1 (cache + rate-limited forced refresh)", got)
	}
}

func TestToUser(t *testing.T) {
	tests := []struct {
		name      string
		res       scimResource
		wantOK    bool
		wantEmail string
		wantName  string
	}{
		{
			name:      "email in userName, store prefix stripped",
			res:       scimResource{UserName: "DEFAULT/dave@wso2.com"},
			wantOK:    true,
			wantEmail: "dave@wso2.com",
			wantName:  "dave",
		},
		{
			name:      "name from userName when no display name",
			res:       scimResource{UserName: "PRIMARY/erin", Emails: []scimEmail{{Value: "erin@wso2.com"}}},
			wantOK:    true,
			wantEmail: "erin@wso2.com",
			wantName:  "erin",
		},
		{
			name:   "no email → dropped",
			res:    scimResource{UserName: "PRIMARY/frank"},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, ok := toUser(tt.res)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if u.Email != tt.wantEmail {
				t.Errorf("email = %q, want %q", u.Email, tt.wantEmail)
			}
			if u.Name != tt.wantName {
				t.Errorf("name = %q, want %q", u.Name, tt.wantName)
			}
		})
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return -1
	}
	return n
}
