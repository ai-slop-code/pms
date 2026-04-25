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
# The file is regenerated on every start so changing the env var and
# restarting the container is enough — no image rebuild, no volume mount.

set -eu

CONFIG_FILE="/usr/share/nginx/html/config.js"
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

if [ -n "$API_BASE_URL" ]; then
    echo "pms-frontend: apiBaseUrl set to ${API_BASE_URL}"
else
    echo "pms-frontend: apiBaseUrl empty (same-origin mode)"
fi
