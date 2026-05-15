import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import "./App.css";
import { AuthProvider, useAuth } from "./auth";
import BillEditor from "./pages/BillEditor";
import FriendSplit from "./pages/FriendSplit";
import Home from "./pages/Home";
import SignIn from "./pages/SignIn";
import Verify from "./pages/Verify";

function ProtectedHome() {
  const { user, loading } = useAuth();
  if (loading) {
    return (
      <main className="app">
        <h1>IOU</h1>
        <p>Loading…</p>
      </main>
    );
  }
  return user ? <Home /> : <Navigate to="/signin" replace />;
}

function ProtectedBillEditor() {
  const { user, loading } = useAuth();
  if (loading) {
    return (
      <main className="app">
        <h1>IOU</h1>
        <p>Loading…</p>
      </main>
    );
  }
  return user ? <BillEditor /> : <Navigate to="/signin" replace />;
}

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<ProtectedHome />} />
          <Route path="/bills/:id" element={<ProtectedBillEditor />} />
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
