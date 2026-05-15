import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api";
import { useAuth } from "../auth";

export default function Verify() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { setUser } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const ran = useRef(false);

  useEffect(() => {
    if (ran.current) return;
    ran.current = true;

    const token = params.get("token");
    if (!token) {
      setError("Missing token.");
      return;
    }
    api
      .verify(token)
      .then((user) => {
        setUser(user);
        navigate("/", { replace: true });
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "verification failed");
      });
  }, [params, navigate, setUser]);

  return (
    <main className="app">
      <h1>splitit</h1>
      {error ? <p className="error">{error}</p> : <p>Signing you in…</p>}
    </main>
  );
}
