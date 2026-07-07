// Command gdrive-auth runs the Google OAuth consent flow once and prints a
// refresh token for storage.gdrive.oauth in config.yaml. The app then acts as
// the consenting user for all Drive access — which, unlike a service account,
// carries personal Drive quota and inherits the user's access to corporate
// Shared Drives.
//
// Usage:
//
//	go run ./cmd/gdrive-auth -client-id <id> -client-secret <secret>
//	go run ./cmd/gdrive-auth -client-secret-file <downloaded-oauth-client.json>
//
// Use the same **Web application** OAuth client as cmd/gdrive-grant, and add the
// loopback redirect (http://127.0.0.1:8899/callback) to its Authorized redirect
// URIs. Sharing the client + user with the Picker grant is what makes the folder
// grant apply to this refresh token. Open the printed URL, consent as the Drive
// user, and the refresh token is printed back here.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

func main() {
	clientID := flag.String("client-id", "", "OAuth client ID (Desktop app)")
	clientSecret := flag.String("client-secret", "", "OAuth client secret")
	secretFile := flag.String("client-secret-file", "", "path to the downloaded OAuth client JSON (alternative to -client-id/-client-secret)")
	port := flag.Int("port", 8899, "local port for the OAuth loopback redirect")
	flag.Parse()

	id, secret := *clientID, *clientSecret
	if *secretFile != "" {
		var err error
		id, secret, err = readClientSecret(*secretFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read client secret file:", err)
			os.Exit(1)
		}
	}
	if id == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "provide -client-id and -client-secret, or -client-secret-file")
		os.Exit(1)
	}

	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", *port)
	conf := &oauth2.Config{
		ClientID:     id,
		ClientSecret: secret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirect,
		// drive.file — matches the server and the Picker grant. Fine-grained: the
		// token can only reach app-created files + folders granted via the Picker.
		Scopes: []string{drive.DriveFileScope},
	}

	const state = "purchasing-gdrive-auth"
	// AccessTypeOffline + ApprovalForce guarantee a refresh_token is returned even
	// if this account previously consented.
	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen on %s: %v\n", redirect, err)
		os.Exit(1)
	}

	fmt.Println("Open this URL in a browser signed in as the Drive user:")
	fmt.Println()
	fmt.Println("  " + authURL)
	fmt.Println()
	fmt.Println("Waiting for consent on " + redirect + " ...")

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if s := r.URL.Query().Get("state"); s != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch: got %q", s)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			http.Error(w, "consent error: "+e, http.StatusBadRequest)
			errCh <- fmt.Errorf("consent error: %s", e)
			return
		}
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "Authorization received. You can close this tab and return to the terminal.")
		codeCh <- code
	})
	go srv.Serve(ln)

	ctx := context.Background()
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = srv.Close()

	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		fmt.Fprintln(os.Stderr, "exchange code:", err)
		os.Exit(1)
	}
	if tok.RefreshToken == "" {
		fmt.Fprintln(os.Stderr, "no refresh token returned — revoke prior access at https://myaccount.google.com/permissions and retry")
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Success. Add this to config.yaml under storage.gdrive:")
	fmt.Println()
	fmt.Println("  auth: \"oauth\"")
	fmt.Println("  oauth:")
	fmt.Printf("    client_id: %q\n", id)
	fmt.Printf("    client_secret: %q\n", secret)
	fmt.Printf("    refresh_token: %q\n", tok.RefreshToken)
}

// readClientSecret extracts the client id/secret from a downloaded Google OAuth
// client JSON (either the "installed" or "web" top-level key).
func readClientSecret(path string) (id, secret string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var doc struct {
		Installed *struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
		Web *struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"web"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", "", err
	}
	switch {
	case doc.Installed != nil:
		return doc.Installed.ClientID, doc.Installed.ClientSecret, nil
	case doc.Web != nil:
		return doc.Web.ClientID, doc.Web.ClientSecret, nil
	default:
		return "", "", fmt.Errorf("no \"installed\" or \"web\" client in %s", path)
	}
}
