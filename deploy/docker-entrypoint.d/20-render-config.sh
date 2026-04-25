#!/bin/sh
# Rewrite /usr/share/nginx/html/config.js (and the CSP in index.html) from
# environment variables at container startup. This lets a single pre-built
# image be retargeted at different backends without rebuilding.
#
# Recognised env vars:
#   PMS_API_BASE_URL   Absolute URL of the backend (e.g. https://api.example.com).
#                      Leave unset/empty for same-origin deployments where this
#                      container's nginx already proxies /api/* to pms-backend.
#                      When set, the script also adds the URL's origin to the
#                      `connect-src` directive of the CSP in index.html so the
#                      browser is allowed to make XHR/fetch calls to it.
#
# Both files are regenerated from pristine templates on every start, so the
# script is idempotent and safe to re-run.

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

cat > "$CONFIG_FILE" <<EOF
// Generated at container start by 20-render-config.sh.
// To change the backend URL, restart the container with a different
// PMS_API_BASE_URL value — no image rebuild required.
window.__PMS_CONFIG__ = {
  apiBaseUrl: '${escaped}',
};
EOF

# Regenerate index.html from the pristine template baked at build time so
# repeated container restarts don't append duplicate connect-src entries.
if [ -f "$INDEX_TEMPLATE" ]; then
    cp "$INDEX_TEMPLATE" "$INDEX_FILE"
fi

if [ -n "$API_BASE_URL" ]; then
    # Strip path/query/fragment to get just scheme://host[:port]. The CSP
    # connect-src directive matches origins, so a path component would be
    # ignored and is misleading. POSIX shell parameter expansion is enough.
    rest="${API_BASE_URL#*://}"            # host[:port]/path?query
    host_port="${rest%%/*}"                # host[:port]
    scheme="${API_BASE_URL%%://*}"         # http or https
    api_origin="${scheme}://${host_port}"

    # Refuse to inject obvious garbage (e.g. an unset variable echoed back).
    case "$api_origin" in
        http://*|https://*) ;;
        *)
            echo "pms-frontend: PMS_API_BASE_URL='${API_BASE_URL}' is not an absolute http(s) URL; ignoring CSP patch" >&2
            api_origin=""
            ;;
    esac

    if [ -n "$api_origin" ]; then
        # Anchor the substitution on `connect-src 'self'` so we don't touch
        # any other directive. Use | as the sed delimiter because the value
        # contains slashes. Avoid `sed -i` to stay portable across BSD/GNU.
        tmp="${INDEX_FILE}.tmp"
        sed "s|connect-src 'self'|connect-src 'self' ${api_origin}|" "$INDEX_FILE" > "$tmp"
        mv "$tmp" "$INDEX_FILE"
        echo "pms-frontend: apiBaseUrl set to ${API_BASE_URL} (CSP connect-src += ${api_origin})"
    fi
else
    echo "pms-frontend: apiBaseUrl empty (same-origin mode)"
fi
