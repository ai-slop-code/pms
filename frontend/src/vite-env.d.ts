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

