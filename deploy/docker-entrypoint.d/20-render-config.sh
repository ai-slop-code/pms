#!/bin/sh
# Rewrite /usr/share/nginx/html/config.js from environment variables at
# container startup. This lets a single pre-built image be retargeted at
# different backends without rebuilding.
#
# Recognised env vars:
#   PMS_API_BASE_URL   Absolute URL of the backend (e.g. https://api.example.com).
#                      Leave unset/empty for same-origin deployments where this
#                      container's nginx already proxies /api/* to pms-backend.
#
# The Content-Security-Policy is built from apiBaseUrl by config.js itself
# at runtime in the browser (via document.write so it flows through the
# HTML parser like a static meta tag), so this script no longer needs to
# touch index.html. index.html is still restored from a pristine template
# on each start so an older image's in-place edits don't survive an
# upgrade.

set -eu

CONFIG_FILE="/usr/share/nginx/html/config.js"
INDEX_FILE="/usr/share/nginx/html/index.html"
INDEX_TEMPLATE="/etc/pms-frontend/index.html.tmpl"
API_BASE_URL="${PMS_API_BASE_URL:-}"

# Escape backslashes, single quotes, and any stray newlines so the value is
# safe to inline as a JS single-quoted string literal.
escaped=$(printf '%s' "$API_BASE_URL" \
  | sed -e 's/\\/\\\\/g' -e "s/'/\\\\'/g" \
  | tr -d '\n\r')

cat > "$CONFIG_FILE" <<JS
// Generated at container start by 20-render-config.sh.
// To change the backend URL, restart the container with a different
// PMS_API_BASE_URL value — no image rebuild required. The CSP meta tag
// is installed at runtime from apiBaseUrl below, so no post-build edit
// of index.html is required for cross-origin setups.
;(function () {
  var config = { apiBaseUrl: '${escaped}' }
  window.__PMS_CONFIG__ = config

  var connectSrc = "'self'"
  var raw = (config.apiBaseUrl || '').trim()
  if (raw) {
    try {
      var u = new URL(raw, window.location.href)
      var origin = u.protocol + '//' + u.host
      if (origin !== window.location.origin) {
        connectSrc += ' ' + origin
      }
    } catch (_) {}
  }
  var policy = [
    "default-src 'self'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    'connect-src ' + connectSrc,
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'"
  ].join('; ')
  document.write(
    '<meta http-equiv="Content-Security-Policy" content="' +
      policy.replace(/"/g, '&quot;') +
      '">'
  )
})()
JS

if [ -f "$INDEX_TEMPLATE" ]; then
    cp "$INDEX_TEMPLATE" "$INDEX_FILE"
fi

if [ -n "$API_BASE_URL" ]; then
    echo "pms-frontend: apiBaseUrl set to ${API_BASE_URL} (CSP installed by config.js at runtime)"
else
    echo "pms-frontend: apiBaseUrl empty (same-origin mode)"
fi
