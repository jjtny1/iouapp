# CLAUDE.md

## Quick Reference

```bash
/boris <task>        # Full workflow
/session-start       # Load context
/session-end         # Save context
/verify-all          # Run checks
/commit-push-pr      # Git workflow
/undo                # Revert change
/checkpoint [name]   # Save point
/fix-issue <num>     # Fix issue
```

## Project

**What**: IOU — split a restaurant bill with friends. The host uploads a
receipt photo, it's parsed into line items/tax/tip, friends open a share link
and claim what they ordered, and each settles their prorated share.

**Stack**: Go (`net/http` + SQLite via pure-Go `modernc.org/sqlite`) single
binary that also serves the SPA; React + TypeScript + Vite frontend in `web/`.
Packages: `internal/{api,auth,db,config,receipt,payment,split}`.

## Commands

| Command                                           | Description                      |
| ------------------------------------------------- | -------------------------------- |
| `IOU_DEV=1 go run ./cmd/server`                   | Start API on :8080 (dev mode)    |
| `cd web && npm run dev`                           | Start Vite dev server on :5173   |
| `go test ./...`                                   | Run Go tests                     |
| `cd web && npm run build`                         | Build the frontend to `web/dist` |
| `cd web && npx tsc -p tsconfig.app.json --noEmit` | Type-check the frontend          |

Receipt parsing uses the Anthropic vision API and needs `ANTHROPIC_API_KEY`;
without it the app falls back to `receipt.StubParser` (a fixed sample receipt).

---

## End-to-End Testing

The Go server also serves the built SPA, so for a full manual test build the
frontend (`cd web && npm run build`) and hit the Go server directly — no Vite
proxy needed. Run on a non-default port if other agents may be using `:8080`:

```bash
IOU_DEV=1 PORT=8099 IOU_BASE_URL=http://localhost:8099 \
  IOU_DB=/tmp/iou-test.db ANTHROPIC_API_KEY=sk-ant-... \
  go run ./cmd/server
```

- **Magic-link sign-in via API.** `POST /api/auth/request {"email":...}` returns
  the dev link in its JSON (`IOU_DEV=1`). Extract the `token` from it and
  `POST /api/auth/verify {"token":...}` with a curl cookie jar (`-c`/`-b`) to
  get a session. The browser SignIn page shows the same dev link on screen.
- **Receipt upload can't be driven through the browser.** The Chrome extension
  blocks programmatic file uploads (`file_upload` → `Not allowed`), so test the
  upload via the API: convert the HEIC first
  (`sips -s format jpeg -Z 1600 in.heic --out out.jpg` on macOS — mirrors what
  `prepareReceiptImage` does client-side), then
  `curl -b cookies -F 'receipt=@out.jpg;type=image/jpeg' /api/bills/{id}/receipt`.
  The browser is still fine for verifying _rendered_ pages.
- The receipt endpoint authorizes by host user id, so a fresh API login as the
  same email can upload to a bill a browser session created.

---

## Mistakes to Avoid

- **Don't use `heic2any` for HEIC conversion.** Its bundled libheif is outdated
  and fails on modern iPhone HEICs with `ERR_LIBHEIF format not supported`. Use
  `heic-to` instead.
- **Don't put `capture="environment"` on the receipt file input.** It forces
  camera-only capture — mobile users can't pick a saved photo, and it breaks
  programmatic uploads.
- **Don't send HEIC to the Anthropic vision API.** It accepts only JPEG, PNG,
  GIF, and WebP. iPhone photos are HEIC and must be converted client-side first.
- **Don't assume the Go server loads a `.env` file** — it does not. Pass
  `ANTHROPIC_API_KEY` inline on every start, or parsing silently falls back to
  the stub parser.
- **Don't use Node < 20.** The Bash tool snapshots PATH at session start; to use
  Node 20, prefix commands with
  `export NVM_DIR="$HOME/.nvm"; source "$NVM_DIR/nvm.sh"; nvm use 20 >/dev/null 2>&1;`
- **Don't commit straight to `main`.** Branch first, then open a PR.
- **Don't rename the Go module path while other worktree branches are in
  flight.** Renaming `module` in `go.mod` rewrites every
  `import "github.com/jjtny1/splitit/..."` line repo-wide — it's all-or-nothing.
  Worktrees are isolated on disk so it won't break a sibling branch's build
  immediately, but at merge time it conflicts with every Go file another branch
  touched, and once it lands on `main` any branch still on the old path stops
  compiling. Do module-path renames as a standalone change when the repo is
  quiet (no other open branches). Note the module path is a _logical_
  identifier — it need not match the on-disk directory or the GitHub repo name,
  so there's no functional pressure to rename it at all.
- **Don't give items a `qty` field.** One `items` row is exactly one claimable
  unit; `price_cents` is that unit's full price. Multi-quantity receipt lines
  are expanded at parse time (see below), so nothing downstream multiplies by a
  quantity.

## Learned Patterns

- **Receipt images are normalized client-side** in `web/src/image.ts`
  (`prepareReceiptImage`): HEIC/HEIF → JPEG, plus downscaling large photos to
  ≤1600px. The heavy libheif WASM in `heic-to` is lazy-loaded via dynamic
  `import()` so only HEIC uploaders pay the bundle cost.
- **The backend validates upload media type** in `internal/api/bills.go` and
  returns a clear `415` for unsupported formats.
- **Receipt parsing is behind a `Parser` interface** (`internal/receipt`):
  `ClaudeParser` when an API key is set, `StubParser` otherwise — keeps the full
  flow testable offline.
- **Payments are behind a `payment.Provider` interface** — `MockProvider` is
  active for v1; real x402/USDC settlement is the planned next step.
- **Money is integer cents end to end.** Tax/tip are prorated with the
  largest-remainder method so totals reconcile to the exact cent.
- **Bills carry a currency.** Each bill has an ISO 4217 `currency` (default
  `USD`). The receipt parser detects it from the image; the host can override
  it in the editor. `*_cents` values are always hundredths of that currency's
  major unit, for _every_ currency (¥4100 → `410000`) — the frontend's
  `formatMoney` (`web/src/money.ts`) uses `Intl.NumberFormat` to render the
  right symbol and fraction digits. Currency-code validation lives in
  `internal/money` (`NormalizeCurrency`).
- **Payment settlement currency is separate from the bill currency.** Payments
  settle in `payment.Currency` (`USDC`); the `payments.currency` column is the
  settlement coin, not the bill's. FX conversion from a non-USD bill currency
  to the settlement currency is intentionally deferred to the x402 work.
- **One item row = one claimable unit.** A receipt line with quantity N>1 is
  expanded at parse time by `receipt.Flatten` into N separate `qty=1` items
  named `Name (1 of N)` … `(N of N)`, each at the per-unit price. This lets
  each friend claim their own unit (e.g. two people each pick one of two
  Cokes) instead of sharing a single multi-quantity checkbox. The `items`
  table has no `qty` column.
- **Auth is magic-link.** In `IOU_DEV=1` the link is returned in the JSON
  response; otherwise it's only logged server-side (no email delivery yet).
- **The verify page can race the auth bootstrap.** `AuthProvider`'s initial
  `GET /api/auth/me` (run unauthenticated on first paint) can resolve _after_
  `Verify` sets the user and clobber it back to `null`, bouncing to `/signin`.
  A full page reload of `/` after the cookie is set re-authenticates cleanly.
  Known issue, not yet fixed.
