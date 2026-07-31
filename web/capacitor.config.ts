import type { CapacitorConfig } from "@capacitor/cli";

// Capacitor wraps the built Vite SPA (web/dist) in a native iOS shell. The
// WebView is served the bundled assets from capacitor://localhost, so the
// build that feeds `cap sync` must set VITE_API_BASE to the live backend
// (see `build:ios` in package.json and vite-env.d.ts).
const config: CapacitorConfig = {
  appId: "ai.iouapp.app",
  appName: "IOU",
  webDir: "dist",
};

export default config;
