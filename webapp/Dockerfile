# Choreo Web Application — Dockerfile build.
#
# WHY THIS EXISTS: the static-web-app buildpack serves dist/ from its own server
# and ignores public/nginx.conf, so none of the security headers in that file
# were ever actually sent and no reverse proxy could take effect. Building the
# component from this Dockerfile instead puts our own nginx in front of the SPA,
# which:
#
#   * serves the CSP / frame / referrer / HSTS / permissions headers,
#   * proxies /api to the backend, making the API same-origin — which is what
#     lets the session cookie be a first-party SameSite=Lax cookie that works in
#     every browser, Safari included, instead of a partitioned third-party one
#     (see docs/sessions.md).
#
# After switching the component to this Dockerfile, set apiBaseUrl to '' in the
# mounted config.js and session.cookie_samesite to "lax" in the backend's
# config.yaml. See docs/deployment-guide.md → "Frontend — Choreo web app".

# ---- build ----------------------------------------------------------------
FROM node:20-alpine AS build
WORKDIR /app

# Dependencies from the lockfile first, so the layer caches across source-only
# changes. `npm ci` (not install) keeps the build reproducible.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# ---- serve ----------------------------------------------------------------
# nginx-unprivileged: listens on 8080 and writes its temp/pid files under /tmp,
# so it runs fine as an arbitrary non-root UID. Choreo requires the container
# user to be non-root with a UID in the 10000-20000 range.
FROM nginxinc/nginx-unprivileged:1.27-alpine

USER root
COPY --from=build /app/dist /usr/share/nginx/html
COPY public/nginx.conf /etc/nginx/conf.d/default.conf
# nginx.conf lives in public/, so Vite also copies it into dist/ — delete that
# copy rather than serving our proxy target and header policy to the internet.
RUN rm -f /usr/share/nginx/html/nginx.conf \
    && chown -R 10014:10014 /usr/share/nginx/html /etc/nginx/conf.d

USER 10014
EXPOSE 8080

# config.js is mounted over the built one at runtime by Choreo
# (DevOps → Configs & Secrets → Config File → /usr/share/nginx/html/config.js).
