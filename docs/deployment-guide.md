# Deployment guide

Operational setup for deploying the purchasing app. This guide currently covers
the **Google account configuration** only:

- [Google Drive — document storage](#google-drive--document-storage)
- [Gmail — outgoing mail](#gmail--outgoing-mail)

Other deployment topics (database, Choreo build/runtime config, OIDC) are covered
elsewhere and will be folded in here later. For the internals behind Drive
storage see [file-storage.md](file-storage.md); this guide is the step-by-step
"how to set up the Google side".

---

## Google Drive — document storage

The app stores document files (quotations, contracts, GRNs, invoices, signed
PDFs, attachments) in a Google Drive folder — one subfolder per PR. In production
use the **OAuth mode** (`storage.backend: "gdrive"`, `gdrive.auth: "oauth"`): the
app acts as a **real user** using the fine-grained **`drive.file`** scope, so it
can only ever touch the folders and files it creates plus the one base folder an
admin grants it via the Google Picker — nothing else in that Drive. This is the
least-privilege option and it sidesteps the service-account "no personal quota"
trap (see [file-storage.md](file-storage.md#gdrive--internalstoragegdrivego) for
why).

### 1. Create / choose a Google Cloud project

1. In the [Google Cloud console](https://console.cloud.google.com/), create or
   select a project owned by the WSO2 Workspace org.
2. Enable two APIs (APIs & Services → Library):
   - **Google Drive API** — file storage.
   - **Google Picker API** — the in-app "Choose folder" grant flow.

### 2. Configure the OAuth consent screen

APIs & Services → OAuth consent screen:

- **User type: Internal.** Because the project lives under the WSO2 Workspace org,
  Internal means no Google verification and the refresh token does not expire on a
  testing schedule. (Only fall back to **External + test users** if the project
  can't be Internal — those tokens expire and must be periodically reconnected.)
- Scope: the app requests only **`.../auth/drive.file`**, a *non-sensitive* scope,
  so no restricted-scope security review is required.

### 3. Create the OAuth client ID and API key

APIs & Services → Credentials:

1. **Create Credentials → OAuth client ID → Web application.**
   - **Authorized JavaScript origins:** the app's own deployed frontend origin
     (e.g. `https://<app>.choreoapps.dev`). Add `http://localhost:5173` too if
     admins will connect Drive from a local dev instance, and `http://localhost:8899`
     if you'll use the CLI bootstrap tool.
   - **Authorized redirect URIs:** the in-app connect flow uses
     `redirect_uri: "postmessage"` and needs **no** redirect URI. Add
     `http://127.0.0.1:8899/callback` only if you'll use the `gdrive-auth` CLI tool.
   - Note the generated **Client ID** and **Client secret**.
2. **Create Credentials → API key.** This is the Picker developer key. Restrict it
   to the **Google Picker API** (and optionally to your app's HTTP referrers).

### 4. Put the static artifacts in `config.yaml`

Only the static GCP artifacts go in config — the *account* and *folder* are set at
runtime from the app UI (below).

```yaml
storage:
  backend: "gdrive"
  gdrive:
    auth: "oauth"
    oauth:
      client_id: "<oauth-web-client-id>"
      client_secret: "<oauth-web-client-secret>"
      api_key: "<google-api-key>"          # Picker developer key
      app_id: ""                           # optional; defaults to the client-id prefix (project number)

# Required once Drive is connected from the UI — encrypts the refresh token at
# rest (AES-256-GCM). Generate with: head -c 32 /dev/urandom | base64
security:
  secret_key: "<base64-32-bytes>"
```

> On Choreo these values are supplied through the runtime `config.yaml` /
> config-map, not committed to the repo (`config.yaml` is gitignored).

### 5. Connect the account and pick the folder (in-app, recommended)

Deploy the app with the config above, then as an **admin** go to
**Settings → File storage**:

1. **Connect Google account** — runs OAuth consent in a popup. The backend
   exchanges the auth code for a **refresh token** and stores it encrypted
   (keyed by `security.secret_key`).
2. **Choose folder** — opens the Google Picker (My Drive + Shared Drive tabs).
   Selecting a folder grants the app `drive.file` access to *that folder only*.
   The backend validates the folder (must be resolvable and writable) and
   hot-swaps the live store — **no restart needed**.

Status flips to **● Connected & healthy**. The refresh token is long-lived;
reconnect only if it's revoked or the folder grant is removed.

> **CLI bootstrap alternative (headless):** if you can't use the UI, mint the
> folder grant + refresh token with `backend/cmd/gdrive-grant` and
> `backend/cmd/gdrive-auth`, then paste `base_folder_id` + `refresh_token` into the
> `oauth:` block. See [file-storage.md → Option B](file-storage.md#option-b--cli-bootstrap-headless).

---

## Gmail — outgoing mail

The app sends notification email (approval requests, team-lead approvals, budget /
legal / security assignee notices and reminders) over plain SMTP. When `email.enabled`
is `false` (the default) messages are only logged, so dev needs no mail server. For a
real deployment, point it at **Gmail's SMTP relay** authenticated as a WSO2 Workspace
account.

### 1. Choose the sending account

Use a dedicated Workspace mailbox as the sender — e.g. `purchasing@wso2.com`. The
`from_address` must be an address that the authenticated account is allowed to send
as (the account itself, or a configured **Send-as** alias in Gmail settings).

### 2. Create an App Password

Gmail SMTP does not accept the normal account password; you need an **App Password**,
which requires 2-Step Verification on the account:

1. Sign in as the sending account and enable **2-Step Verification**
   (myaccount.google.com → Security).
2. Go to **App passwords** (myaccount.google.com/apppasswords), create one named
   e.g. "purchasing-app", and copy the 16-character password.

> If your Workspace admin has disabled App Passwords, use the **Google Workspace
> SMTP relay** service (`smtp-relay.gmail.com`) instead — same host/port shape,
> authorized by IP/domain in the Admin console rather than by app password.

### 3. Configure `config.yaml`

```yaml
email:
  enabled: true
  smtp_host: "smtp.gmail.com"
  smtp_port: 587                       # STARTTLS
  username: "purchasing@wso2.com"      # the sending account
  password: "<16-char-app-password>"   # App Password, not the login password
  from_address: "purchasing@wso2.com"  # must be send-as-authorized for username
  app_base_url: "https://<app>.choreoapps.dev"   # frontend origin for links in emails
```

Notes:

- Port **587** with STARTTLS is what the SMTP mailer uses (`net/smtp` PlainAuth
  upgrades the connection via STARTTLS automatically). Port 465 (implicit TLS) is
  not supported by this mailer.
- `app_base_url` is the origin used to build clickable links inside the emails; if
  omitted it defaults to the first CORS allowed origin. Set it explicitly to the
  deployed frontend origin.
- Sends are **best-effort** — a failing mail server never blocks or fails the user
  action that triggered the notification; failures are logged. To verify delivery,
  trigger an approval-assignee notification and check the recipient's inbox (and the
  backend log line `email: SMTP mailer enabled` on startup).
- Keep `password` out of git — supply it through the runtime config / secret on
  Choreo, never commit `config.yaml`.
