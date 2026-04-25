/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * Base URL for backend API requests. Leave empty for same-origin
   * deployments (the reverse proxy handles `/api/*`). Set to an absolute
   * URL like `https://api.pms.airport.sk` for cross-origin deployments.
   */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

/**
 * Runtime configuration injected by `public/config.js` (loaded from
 * `index.html` before the app bundle). Operators can edit that file after
 * extracting the release tarball to retarget the API without rebuilding.
 */
interface PmsRuntimeConfig {
  apiBaseUrl?: string
}

declare global {
  interface Window {
    __PMS_CONFIG__?: PmsRuntimeConfig
  }
}

export {}

