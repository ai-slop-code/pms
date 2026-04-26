// Runtime configuration for the PMS frontend.
//
// This file is loaded by index.html *before* the bundled application, so
// changes take effect on the next browser refresh — no rebuild required.
// Edit the values below after extracting the release tarball to point the
// SPA at the backend that suits your deployment.
//
// apiBaseUrl
//   ""                          Same-origin. The host serving these static
//                               files must reverse-proxy /api/* to the
//                               backend. This is the recommended setup.
//   "https://api.example.com"   Cross-origin. The backend lives on its own
//                               domain. You must also:
//                                 - set CORS_ORIGINS on the backend to the
//                                   SPA's origin,
//                                 - set PMS_COOKIE_SAMESITE=none and
//                                   PMS_COOKIE_SECURE=true on the backend.
//                               The Content-Security-Policy meta tag is
//                               installed below from this same value, so
//                               you do NOT have to edit index.html.
;(function () {
  var config = {
    apiBaseUrl: '',
  }
  window.__PMS_CONFIG__ = config

  // Build the CSP. `connect-src` always includes 'self'; if the operator
  // pointed apiBaseUrl at a different origin, allow that origin too.
  // Everything else stays locked down so the SPA can't be repurposed to
  // pull from arbitrary third parties.
  var connectSrc = "'self'"
  var raw = (config.apiBaseUrl || '').trim()
  if (raw) {
    try {
      var u = new URL(raw, window.location.href)
      var origin = u.protocol + '//' + u.host
      if (origin !== window.location.origin) {
        connectSrc += ' ' + origin
      }
    } catch (_) {
      // Malformed URL — leave connect-src at 'self' rather than emit a
      // broken policy. The app will surface the configuration error when
      // it tries to fetch from the bad value.
    }
  }
  var policy = [
    "default-src 'self'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    'connect-src ' + connectSrc,
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
  ].join('; ')
  // Use document.write so the meta tag is injected into the HTML parser's
  // input stream and is processed exactly like a static <meta> baked into
  // index.html. Dynamically appending via appendChild() also works in
  // modern browsers but is loosely specced; document.write is well-defined
  // for synchronous classic scripts running during parsing, which is how
  // config.js is loaded.
  document.write(
    '<meta http-equiv="Content-Security-Policy" content="' +
      policy.replace(/"/g, '&quot;') +
      '">'
  )
})()
