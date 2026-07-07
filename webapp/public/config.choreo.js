// Choreo deployment: mount this file via
// Choreo Console → DevOps → Configs & Secrets → Config File
// Mount path: /usr/share/nginx/html/config.js
//
// Fill in the actual values before mounting. This overrides the
// build-time VITE_* env vars at runtime without requiring a rebuild.
// Redirect URIs are derived from window.location.origin in the app, so
// only these three values are needed here.

window.config = {
  apiBaseUrl: 'https://<choreo-backend-url>',
  oidcAuthority: 'https://api.asgardeo.io/t/wso2/oauth2/token',
  oidcClientId: '<your-asgardeo-client-id>',
};
