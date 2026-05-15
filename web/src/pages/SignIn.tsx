import { useState, type FormEvent } from "react";
import { api } from "../api";

export default function SignIn() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [devLink, setDevLink] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const res = await api.requestLink(email);
      setSent(true);
      setDevLink(res.link ?? null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "request failed");
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <main className="app">
        <h1>IOU</h1>
        <p>Check your email for a sign-in link.</p>
        {devLink && (
          <p>
            Dev link: <a href={devLink}>{devLink}</a>
          </p>
        )}
      </main>
    );
  }

  return (
    <main className="app">
      <h1>IOU</h1>
      <p>Sign in to split the bill with friends.</p>
      <form onSubmit={onSubmit}>
        <input
          type="email"
          required
          placeholder="you@example.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <button type="submit" disabled={busy}>
          {busy ? "Sending…" : "Send magic link"}
        </button>
      </form>
      {error && <p className="error">{error}</p>}
    </main>
  );
}
