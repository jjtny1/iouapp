import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Capacitor } from "@capacitor/core";
import "./index.css";
import App from "./App.tsx";

// In the native iOS app the WebView should behave like a native screen: no
// pinch-zoom, and no auto-zoom when a sub-16px input is focused. iOS zooms
// into such inputs, and an app-switch mid-zoom (tapping the magic-link email,
// then returning) leaves the WebView stuck zoomed in. Disabling user scaling
// only on native fixes that while keeping the web app fully zoomable.
if (Capacitor.isNativePlatform()) {
  document
    .querySelector('meta[name="viewport"]')
    ?.setAttribute(
      "content",
      "width=device-width, initial-scale=1.0, viewport-fit=cover, " +
        "maximum-scale=1.0, user-scalable=no",
    );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
