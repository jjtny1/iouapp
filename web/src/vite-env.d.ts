/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * Base URL for the IOU API. Empty for the web build — the Go server serves
   * the SPA, so `/api` is same-origin. The native iOS (Capacitor) build sets
   * this to the live backend (https://iouapp.ai), since its WebView is served
   * from capacitor://localhost and `/api` would otherwise resolve there.
   */
  readonly VITE_API_BASE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
