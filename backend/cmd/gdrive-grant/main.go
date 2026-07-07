// Command gdrive-grant performs the one-time Google Picker grant that gives the
// app fine-grained (drive.file) access to a single base folder — including one
// inside a corporate Shared Drive — without ever granting access to the rest of
// the user's Drive.
//
// It serves a tiny local page that runs the Google Picker. Sign in as the Drive
// user, select the base folder, and the page cements the grant (a files.get with
// the token) and prints the folder ID. Run this once per base folder; the grant
// then applies to the refresh token minted by cmd/gdrive-auth (same OAuth client
// + same user), which the server uses.
//
// Prerequisites (Google Cloud console, same **Web application** OAuth client used
// by cmd/gdrive-auth):
//   - an API key (Picker requires setDeveloperKey)
//   - Authorized JavaScript origin: http://localhost:<port> (default 8899)
//
// Usage:
//
//	go run ./cmd/gdrive-grant -client-id <web-client-id> -api-key <api-key>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"
)

const page = `<!doctype html>
<html><head><meta charset="utf-8"><title>Drive folder grant</title>
<style>body{font:15px system-ui,sans-serif;margin:3rem;max-width:44rem}
button{font-size:15px;padding:.6rem 1rem;cursor:pointer}
pre{background:#f4f4f5;padding:1rem;border-radius:6px;white-space:pre-wrap}</style>
</head><body>
<h2>Grant the purchasing app access to one Drive folder</h2>
<p>Sign in as the Drive user, then pick the <b>base folder</b> (it may live inside a
Shared Drive). Only that folder is granted — nothing else in your Drive.</p>
<button id="go" disabled>Sign in &amp; pick folder</button>
<pre id="out">Loading Google libraries…</pre>
<script>
const CLIENT_ID = {{.ClientID}};
const API_KEY = {{.APIKey}};
const APP_ID = {{.AppID}};
const SCOPE = "https://www.googleapis.com/auth/drive.file";
let tokenClient, accessToken, gapiReady=false, gisReady=false;
const out = (t)=>document.getElementById('out').textContent=t;
function ready(){ if(gapiReady && gisReady){ document.getElementById('go').disabled=false; out('Ready. Click the button.'); } }
function gapiLoaded(){ gapi.load('picker', ()=>{ gapiReady=true; ready(); }); }
function gisLoaded(){
  tokenClient = google.accounts.oauth2.initTokenClient({
    client_id: CLIENT_ID, scope: SCOPE,
    callback: (resp)=>{ if(resp.error){ out('Auth error: '+resp.error); return; } accessToken=resp.access_token; createPicker(); }
  });
  gisReady=true; ready();
}
document.getElementById('go').onclick = ()=>{ out('Requesting access…'); tokenClient.requestAccessToken({prompt:''}); };
function createPicker(){
  // Two tabs: My Drive folders, and Shared Drive folders.
  const myDrive = new google.picker.DocsView(google.picker.ViewId.FOLDERS)
    .setSelectFolderEnabled(true).setIncludeFolders(true).setOwnedByMe(true);
  const shared = new google.picker.DocsView(google.picker.ViewId.FOLDERS)
    .setSelectFolderEnabled(true).setIncludeFolders(true).setEnableDrives(true);
  new google.picker.PickerBuilder()
    .enableFeature(google.picker.Feature.SUPPORT_DRIVES)
    .setAppId(APP_ID)   // project number — links the drive.file grant to this app
    .setDeveloperKey(API_KEY).setOAuthToken(accessToken)
    .addView(myDrive).addView(shared)
    .setTitle('Pick the base folder').setCallback(pickerCallback).build()
    .setVisible(true);
}
async function pickerCallback(data){
  if(data.action !== google.picker.Action.PICKED) return;
  const folder = data.docs[0];
  // Cement the grant + verify create access is usable: read the folder back.
  const url = 'https://www.googleapis.com/drive/v3/files/'+folder.id+
    '?supportsAllDrives=true&fields=id,name,driveId,capabilities(canAddChildren)';
  const r = await fetch(url,{headers:{Authorization:'Bearer '+accessToken}});
  const body = await r.text();
  let meta={}; try{ meta=JSON.parse(body); }catch(e){}
  await fetch('/picked',{method:'POST',headers:{'Content-Type':'application/json'},
    body: JSON.stringify({ok:r.ok, status:r.status, id:folder.id, name:meta.name||'',
      driveId:meta.driveId||'',
      canAddChildren: !!(meta.capabilities && meta.capabilities.canAddChildren),
      errorBody: r.ok ? '' : body})});
  if(r.ok){
    out('Granted: '+(meta.name||folder.id)+'\nfolder id: '+folder.id+
      '\ndriveId: '+(meta.driveId||'(My Drive)')+
      '\ncan create files inside: '+(meta.capabilities && meta.capabilities.canAddChildren)+
      '\n\nDone — return to the terminal. You can close this tab.');
  } else {
    out('files.get failed: HTTP '+r.status+'\nfolder id: '+folder.id+'\n\n'+body+
      '\n\nThe folder id is captured — see the terminal for next steps.');
  }
}
</script>
<script async defer src="https://apis.google.com/js/api.js" onload="gapiLoaded()"></script>
<script async defer src="https://accounts.google.com/gsi/client" onload="gisLoaded()"></script>
</body></html>`

type picked struct {
	OK             bool   `json:"ok"`
	Status         int    `json:"status"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	DriveID        string `json:"driveId"`
	CanAddChildren bool   `json:"canAddChildren"`
	ErrorBody      string `json:"errorBody"`
}

func main() {
	clientID := flag.String("client-id", "", "Web application OAuth client ID")
	apiKey := flag.String("api-key", "", "Google API key (for the Picker)")
	appID := flag.String("app-id", "", "GCP project number (Picker setAppId; default: the client-id prefix)")
	port := flag.Int("port", 8899, "local port (must be an Authorized JavaScript origin on the OAuth client)")
	flag.Parse()
	if *clientID == "" || *apiKey == "" {
		fmt.Fprintln(os.Stderr, "provide -client-id and -api-key")
		os.Exit(1)
	}
	// The app id is the GCP project number — the client id is "<projectnum>-xxxx...".
	if *appID == "" {
		if i := strings.IndexByte(*clientID, '-'); i > 0 {
			*appID = (*clientID)[:i]
		}
	}
	if *appID == "" {
		fmt.Fprintln(os.Stderr, "could not derive -app-id from client-id; pass it explicitly (GCP project number)")
		os.Exit(1)
	}

	tmpl := template.Must(template.New("page").Parse(page))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// template quotes CLIENT_ID/API_KEY as JS string literals safely.
		_ = tmpl.Execute(w, struct{ ClientID, APIKey, AppID template.JS }{
			ClientID: template.JS(fmt.Sprintf("%q", *clientID)),
			APIKey:   template.JS(fmt.Sprintf("%q", *apiKey)),
			AppID:    template.JS(fmt.Sprintf("%q", *appID)),
		})
	})

	done := make(chan picked, 1)
	mux.HandleFunc("/picked", func(w http.ResponseWriter, r *http.Request) {
		var p picked
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		done <- p
	})

	addr := fmt.Sprintf("localhost:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen on %s: %v\n", addr, err)
		os.Exit(1)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	fmt.Printf("Open http://%s in a browser and pick the base folder.\n", addr)
	fmt.Println("(The port must be an Authorized JavaScript origin on the OAuth client.)")

	p := <-done
	ctx := context.Background()
	_ = srv.Shutdown(ctx)

	if !p.OK {
		fmt.Fprintf(os.Stderr, "\nGrant/read failed (HTTP %d) for folder id %s\n", p.Status, p.ID)
		if p.ErrorBody != "" {
			fmt.Fprintln(os.Stderr, p.ErrorBody)
		}
		os.Exit(1)
	}
	fmt.Println("\nGrant complete.")
	fmt.Printf("  base_folder_id: %s\n", p.ID)
	fmt.Printf("  name:           %s\n", p.Name)
	if p.DriveID != "" {
		fmt.Printf("  shared drive:   %s\n", p.DriveID)
	} else {
		fmt.Println("  location:       My Drive (not a Shared Drive)")
	}
	fmt.Printf("  can create files inside: %v\n", p.CanAddChildren)
	if !p.CanAddChildren {
		fmt.Println("\nWARNING: the user lacks create access in this folder — uploads will fail.")
	}
	fmt.Println("\nNext: run `go run ./cmd/gdrive-auth` (same OAuth client, same user) to mint")
	fmt.Println("the refresh token, then set storage.gdrive.base_folder_id to the id above.")
}
