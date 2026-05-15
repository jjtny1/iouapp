import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api";
import { useAuth } from "../auth";
import { Brand, PaperApp } from "../ui";

export default function Verify() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { setUser } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const ran = useRef(false);
  const token = params.get("token");

  useEffect(() => {
    if (ran.current || !token) return;
    ran.current = true;

    api
      .verify(token)
      .then((user) => {
        setUser(user);
        navigate("/", { replace: true });
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "verification failed");
      });
  }, [token, navigate, setUser]);

  const message = !token ? "Missing token." : error;

  return (
    <PaperApp>
      <div
        className="page-center"
        style={{ alignItems: "center", justifyContent: "center" }}
      >
        <Brand size={64} />
        {message ? (
          <p className="body danger mt-6 center">{message}</p>
        ) : (
          <p className="eyebrow mt-6">Signing you in…</p>
        )}
      </div>
    </PaperApp>
  );
}
