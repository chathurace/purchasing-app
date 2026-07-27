// Package directory fetches the org user directory from the connected identity
// server's SCIM2 API and serves it (cached) to power name/email autocomplete.
// When SCIM is not configured, the Service falls back to a supplied source
// (the app's own DB users) so dev runs without a machine-to-machine app.
package directory

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// User is a single directory entry — just what an autocomplete needs. It is not
// necessarily a provisioned app user (they may never have logged in).
type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SCIMConfig configures the SCIM2 client.
type SCIMConfig struct {
	BaseURL            string // SCIM2 base, e.g. https://host:9443/scim2
	TokenURL           string // OAuth2 token endpoint
	ClientID           string
	ClientSecret       string
	Scopes             []string
	InsecureSkipVerify bool
	PageSize           int
}

// scimClient calls the SCIM2 Users endpoint with an auto-refreshing
// client-credentials token.
type scimClient struct {
	usersURL string
	pageSize int
	http     *http.Client // carries the bearer token + (optionally) an insecure transport
}

func newSCIMClient(cfg SCIMConfig) *scimClient {
	base := &http.Client{Timeout: 30 * time.Second}
	if cfg.InsecureSkipVerify {
		base.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev only, opt-in
		}
	}
	// Inject the base client so both the token fetch and the API calls honour the
	// insecure transport (mirrors the go-oidc setup in middleware/auth.go).
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	ccfg := &clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		Scopes:       cfg.Scopes,
	}
	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	return &scimClient{
		usersURL: strings.TrimRight(cfg.BaseURL, "/") + "/Users",
		pageSize: pageSize,
		http:     ccfg.Client(ctx),
	}
}

// scimListResponse is the SCIM2 ListResponse envelope.
type scimListResponse struct {
	TotalResults int            `json:"totalResults"`
	StartIndex   int            `json:"startIndex"`
	ItemsPerPage int            `json:"itemsPerPage"`
	Resources    []scimResource `json:"Resources"`
}

type scimResource struct {
	UserName string `json:"userName"`
	Name     struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
	Emails []scimEmail `json:"emails"`
}

// scimEmail tolerates both shapes SCIM allows: a bare string ("a@b.com") or an
// object ({"value":"a@b.com","primary":true}).
type scimEmail struct {
	Value   string
	Primary bool
}

func (e *scimEmail) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		e.Value = s
		return nil
	}
	var o struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	e.Value, e.Primary = o.Value, o.Primary
	return nil
}

// listAll pages through every user in the directory. It caps iterations so a
// misbehaving server can't loop forever.
func (c *scimClient) listAll(ctx context.Context) ([]User, error) {
	const maxPages = 1000
	var out []User
	startIndex := 1
	for page := 0; page < maxPages; page++ {
		resp, err := c.fetchPage(ctx, startIndex)
		if err != nil {
			return nil, err
		}
		for _, r := range resp.Resources {
			if u, ok := toUser(r); ok {
				out = append(out, u)
			}
		}
		got := len(resp.Resources)
		if got == 0 {
			break
		}
		startIndex += got
		// Stop once we've walked past the reported total (when provided).
		if resp.TotalResults > 0 && startIndex > resp.TotalResults {
			break
		}
	}
	return out, nil
}

func (c *scimClient) fetchPage(ctx context.Context, startIndex int) (*scimListResponse, error) {
	q := url.Values{}
	q.Set("startIndex", fmt.Sprintf("%d", startIndex))
	q.Set("count", fmt.Sprintf("%d", c.pageSize))
	q.Set("attributes", "userName,name.givenName,name.familyName,emails")
	reqURL := c.usersURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scim request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scim users returned %d", res.StatusCode)
	}
	var parsed scimListResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode scim response: %w", err)
	}
	return &parsed, nil
}

// toUser maps a SCIM resource to a directory entry, deriving a display name and
// preferring the primary email. Returns ok=false when no usable email exists.
func toUser(r scimResource) (User, bool) {
	email := primaryEmail(r.Emails)
	if email == "" {
		// Some WSO2 IS deployments carry the email in userName.
		if strings.Contains(r.UserName, "@") {
			email = stripUserStoreDomain(r.UserName)
		}
	}
	if email == "" {
		return User{}, false
	}
	name := strings.TrimSpace(r.Name.GivenName + " " + r.Name.FamilyName)
	if name == "" {
		if uname := stripUserStoreDomain(r.UserName); uname != "" && !strings.Contains(uname, "@") {
			name = uname
		} else if at := strings.IndexByte(email, '@'); at > 0 {
			name = email[:at]
		}
	}
	return User{Name: name, Email: strings.ToLower(strings.TrimSpace(email))}, true
}

func primaryEmail(emails []scimEmail) string {
	first := ""
	for _, e := range emails {
		v := strings.TrimSpace(e.Value)
		if v == "" {
			continue
		}
		if e.Primary {
			return v
		}
		if first == "" {
			first = v
		}
	}
	return first
}

// stripUserStoreDomain removes a WSO2 user-store prefix ("PRIMARY/alice").
func stripUserStoreDomain(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}
