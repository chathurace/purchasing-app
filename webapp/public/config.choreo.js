// Choreo deployment: mount this file via
// Choreo Console → DevOps → Configs & Secrets → Config File
// Mount path: /usr/share/nginx/html/config.js
//
// Fill in the actual values before mounting. This overrides the
// build-time VITE_* env vars at runtime without requiring a rebuild.
// Redirect URIs are derived from window.location.origin in the app, so
// only these values are needed here.

window.config = {
  // Empty = same origin: nginx proxies /api through to the backend (see
  // public/nginx.conf, which is live only on the Dockerfile build). That is what
  // makes the 60-day session cookie a first-party SameSite=Lax cookie, working
  // in every browser including Safari — see docs/sessions.md.
  //
  // Set the absolute backend URL here instead only when serving the SPA from the
  // static-web-app buildpack (no proxy). The app then authenticates with Bearer
  // tokens unless `apiAllowCredentials: true` is also set AND the backend runs
  // session.cookie_samesite: "none" AND the API gateway returns
  // Access-Control-Allow-Credentials.
  apiBaseUrl: '',
  oidcAuthority: 'https://api.asgardeo.io/t/wso2/oauth2/token',
  oidcClientId: '<your-asgardeo-client-id>',
};
