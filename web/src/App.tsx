import { useEffect, type ReactNode } from "react";
import {
  BrowserRouter,
  Navigate,
  Route,
  Routes,
  useNavigate,
} from "react-router-dom";
import { App as CapacitorApp } from "@capacitor/app";
import { Capacitor } from "@capacitor/core";
import "./App.css";
import { AuthProvider, useAuth } from "./auth";
import BillEditor from "./pages/BillEditor";
import FriendSplit from "./pages/FriendSplit";
import Home from "./pages/Home";
import SignIn from "./pages/SignIn";
import Verify from "./pages/Verify";
import { Brand, PaperApp } from "./ui";

function LoadingScreen() {
  return (
    <PaperApp>
      <div
        className="page-center"
        style={{ alignItems: "center", justifyContent: "center" }}
      >
        <Brand size={56} />
        <p className="eyebrow mt-6">Loading…</p>
      </div>
    </PaperApp>
  );
}

function Protected({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <LoadingScreen />;
  return user ? <>{children}</> : <Navigate to="/signin" replace />;
}

// DeepLinkHandler routes Universal Links into the SPA. When a magic-link email
// is tapped on a device with the app installed, iOS opens the app (not Safari)
// and fires `appUrlOpen` with the full https://iouapp.ai/auth/verify?token=…
// URL; we navigate the in-app router to that path so the Verify page signs the
// user in. No-op on the web build, where /auth/verify just loads normally.
function DeepLinkHandler() {
  const navigate = useNavigate();
  useEffect(() => {
    if (!Capacitor.isNativePlatform()) return;
    const handle = CapacitorApp.addListener("appUrlOpen", ({ url }) => {
      try {
        const parsed = new URL(url);
        navigate(parsed.pathname + parsed.search, { replace: true });
      } catch {
        // Ignore payloads that are not URLs.
      }
    });
    return () => {
      void handle.then((h) => h.remove());
    };
  }, [navigate]);
  return null;
}

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <DeepLinkHandler />
        <Routes>
          <Route
            path="/"
            element={
              <Protected>
                <Home />
              </Protected>
            }
          />
          <Route
            path="/bills/:id"
            element={
              <Protected>
                <BillEditor />
              </Protected>
            }
          />
          <Route path="/b/:token" element={<FriendSplit />} />
          <Route path="/signin" element={<SignIn />} />
          <Route path="/auth/verify" element={<Verify />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;
