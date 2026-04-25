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
//                                   PMS_COOKIE_SECURE=true on the backend,
//                                 - add the API origin to the connect-src
//                                   directive of the CSP in index.html
//                                   (default is connect-src 'self').
window.__PMS_CONFIG__ = {
  apiBaseUrl: '',
}
