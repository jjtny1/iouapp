import type { ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
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

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
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
