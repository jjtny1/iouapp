import { useState, type FormEvent } from "react";
import { api } from "../api";
import { Brand, Icon, PaperApp } from "../ui";

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

  return (
    <PaperApp>
      <div className="page-center">
        <div style={{ marginTop: 40, textAlign: "center" }}>
          <Brand size={84} />
        </div>
        <p className="body center mt-4" style={{ fontSize: 15 }}>
          Pay back your friends.
          <br />
          One tap at a time.
        </p>

        {sent ? (
          <div
            className="fade-up center"
            style={{ marginTop: "auto", paddingBottom: 24 }}
          >
            <div
              style={{
                width: 56,
                height: 56,
                borderRadius: "50%",
                background: "var(--accent)",
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "center",
                marginBottom: 12,
                color: "var(--ink)",
              }}
            >
              <Icon.Mail size={22} />
            </div>
            <p className="h-section" style={{ fontSize: 24 }}>
              Check your mail.
            </p>
            <p className="body muted mt-2" style={{ fontSize: 13 }}>
              We sent a link to
              <br />
              <span className="mono">{email}</span>
            </p>
            {devLink && (
              <p className="mt-4" style={{ fontSize: 12 }}>
                <a className="link-btn" href={devLink}>
                  Dev link — open it
                </a>
              </p>
            )}
          </div>
        ) : (
          <form
            onSubmit={onSubmit}
            className="col gap-3"
            style={{ marginTop: "auto", paddingBottom: 24 }}
          >
            <label className="eyebrow" htmlFor="email">
              Email
            </label>
            <input
              id="email"
              className="input"
              type="email"
              required
              placeholder="you@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <button
              type="submit"
              className="btn btn-block mt-2"
              disabled={busy}
            >
              <Icon.Mail size={14} /> {busy ? "Sending…" : "Send magic link"}
            </button>
            {error && (
              <p className="body danger center" style={{ fontSize: 12 }}>
                {error}
              </p>
            )}
            <p
              className="body muted center"
              style={{ fontSize: 11, marginTop: 6 }}
            >
              No passwords. We'll email you a link.
            </p>
          </form>
        )}
      </div>
    </PaperApp>
  );
}
