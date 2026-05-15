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
proxy needed. Pick an uncommon high port — other agents grab `:8080` and even
`:8099`; if the port is taken the server logs `bind: address already in use`
and exits (don't `kill` the squatter, it's another agent — just pick another
port):

```bash
IOU_DEV=1 PORT=8231 IOU_BASE_URL=http://localhost:8231 \
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
- **Don't add VAT-inclusive tax as `tax_cents`.** `tax_cents` is tax _added on
  top_ of item prices (US-style sales tax). European VAT is already baked into
  the printed prices — often shown broken out into several rate lines (`VAT
23%`, `VAT 8%`) for information only. Reporting it as `tax_cents` double-counts
  it and overshoots the receipt total. Two defences enforce this: the parse
  prompt makes the model reconcile its parts against the printed
  `grand_total_cents` before answering, and `receipt.reconcile` (run inside
  `parseReceiptJSON`) re-checks server-side — when items + tip + service already
  equal the printed total it zeroes the tax, and logs any mismatch it can't
  explain. The invariant: item prices + tax + tip + service == printed total.
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
- **Don't build the production Docker image without `--platform linux/amd64`.**
  Apple Silicon Macs build arm64 images by default, but the Fargate task runs
  x86_64 — a native-arch image pushes fine, then the task dies on start with an
  `exec format error`. Always `docker build --platform linux/amd64 …`.
- **Don't let `terraform apply` create the ECS service before an image exists
  in ECR — actually, it's fine.** `aws_ecs_service` does not wait for task
  health unless `wait_for_steady_state = true` (it isn't set), so a single full
  apply succeeds; the service just has no healthy task until you push an image
  and `update-service --force-new-deployment`. No need to stage the apply.

## Learned Patterns

- **Schema changes need a migration, not just `schema.sql`.** `CREATE TABLE IF
NOT EXISTS` never alters an existing table, so a column added only to
  `schema.sql` is missing on any database created before it. Add new columns in
  _two_ places: the `CREATE TABLE` in `internal/db/schema.sql` (for fresh DBs)
  and the `migrations` slice in `internal/db/db.go` as an idempotent
  `ALTER TABLE … ADD COLUMN` (for existing DBs — a `duplicate column` error is
  caught and ignored).
- **Receipt images are normalized client-side** in `web/src/image.ts`
  (`prepareReceiptImage`): HEIC/HEIF → JPEG, plus downscaling large photos to
  ≤1600px. The heavy libheif WASM in `heic-to` is lazy-loaded via dynamic
  `import()` so only HEIC uploaders pay the bundle cost.
- **The backend validates upload media type** in `internal/api/bills.go` and
  returns a clear `415` for unsupported formats.
- **Receipt parsing is behind a `Parser` interface** (`internal/receipt`):
  `ClaudeParser` when an API key is set, `StubParser` otherwise — keeps the full
  flow testable offline.
- **Payments are Venmo hand-offs.** The host saves a `venmo_handle` on their
  user row (set in the bill editor or on the Home page; new tabs reuse it).
  `POST /pay` returns a payment intent — the host's handle, the amount owed,
  and `app_url`/`web_url` deep links built by `internal/payment`. Phones get
  the `venmo://` app link; desktops get a QR code encoding the `web_url`.
  Venmo reports nothing back, so a payment is marked paid by the friend's
  self-report (`POST /pay/confirm`, no proof) or by the host toggling it
  (`POST /bills/{id}/payments/{pid}` with `{"paid":bool}`). The `payments`
  table keeps vestigial `provider`/`tx_ref` columns from the earlier USDC
  design — always written `'venmo'`/`NULL`, never read.
- **Money is integer cents end to end.** Tax, tip and a percent service charge
  are prorated with the largest-remainder method against the bill's _full_ item
  subtotal — `split.prorate` treats the unclaimed items as one extra bucket, so
  a claimer is never charged for items they didn't claim and the amount owed on
  unclaimed items stays in `UnclaimedCents`. Totals still reconcile to the exact
  cent.
- **Service charge is a bill field, never a claimable item.** Each bill has a
  `service_charge_kind` (`none`/`percent`/`fixed`). A `percent` charge stores a
  rate in basis points (`service_charge_rate_bps`, 1250 = 12.5%); its amount is
  derived from the item subtotal at split time (`split.serviceTotal`) so it
  stays correct as items are edited, and it is prorated over claimers like tax.
  A `fixed` charge stores a flat `service_charge_cents` and an optional
  `service_charge_headcount`; it splits evenly across `max(headcount, joined
count)` shares — shares beyond the joined participants go to `unclaimed` so
  totals still reconcile. `split.Compute` takes a `split.Input` struct (not
  positional args) and needs the full participant ID list, since a fixed charge
  is owed even by participants who claimed nothing. The receipt parser detects
  the charge (`ParsedReceipt.ServiceCharge`); `StubParser` returns a 10% one.
- **Bills carry a currency.** Each bill has an ISO 4217 `currency` (default
  `USD`). The receipt parser detects it from the image; the host can override
  it in the editor. `*_cents` values are always hundredths of that currency's
  major unit, for _every_ currency (¥4100 → `410000`) — the frontend's
  `formatMoney` (`web/src/money.ts`) uses `Intl.NumberFormat` to render the
  right symbol and fraction digits. Currency-code validation lives in
  `internal/money` (`NormalizeCurrency`).
- **Venmo payments carry the bill's own currency.** `payments.currency` is the
  bill currency, and the amount in a Venmo link is its raw major-unit value
  (`amount_cents/100`). Venmo settles in USD only, so for a non-USD bill the
  prefilled amount is nominal — FX conversion is intentionally not done.
- **One item row = one claimable unit.** A receipt line with quantity N>1 is
  expanded at parse time by `receipt.Flatten` into N separate `qty=1` items
  named `Name (1 of N)` … `(N of N)`, each at the per-unit price. This lets
  each friend claim their own unit (e.g. two people each pick one of two
  Cokes) instead of sharing a single multi-quantity checkbox. The `items`
  table has no `qty` column.
- **Auth is magic-link.** In `IOU_DEV=1` the link is returned in the JSON
  response. In prod it is emailed: `NewRouter` takes an `auth.EmailSender`,
  chosen by `IOU_MAIL_PROVIDER` — a log-only sender by default, or `SESSender`
  (`internal/auth/ses.go`) when set to `ses`, which sends via Amazon SES from
  `IOU_MAIL_FROM` in `AWS_REGION`.
- **The app is deployed to AWS ECS Fargate** — live at `https://iouapp.ai`.
  `deploy/` holds a 3-stage `Dockerfile` and `deploy/terraform/` (the whole
  stack: VPC, ALB + ACM HTTPS, ECS, ECR, EFS for the SQLite file, Route 53,
  SES). Runbook and teardown: `deploy/README.md`. To ship a new version: build
  `--platform linux/amd64`, push to ECR, then `aws ecs update-service
--cluster iou-cluster --service iou-service --force-new-deployment`. The
  `ANTHROPIC_API_KEY` lives in SSM Parameter Store as a `SecureString` (name
  `/iou/ANTHROPIC_API_KEY`), injected into the container by ECS — never in
  Terraform state or the task definition. `IOU_DEV` is never set in prod. SES
  starts in sandbox mode (only verified recipient addresses receive mail);
  request production access to email arbitrary users.
- **The verify page can race the auth bootstrap.** `AuthProvider`'s initial
  `GET /api/auth/me` (run unauthenticated on first paint) can resolve _after_
  `Verify` sets the user and clobber it back to `null`, bouncing to `/signin`.
  A full page reload of `/` after the cookie is set re-authenticates cleanly.
  Known issue, not yet fixed.
- **Editing a bill after friends have claimed fails.** `saveBillAndItems` (used
  by `PATCH /api/bills/{id}` and receipt re-upload) deletes and recreates every
  `items` row; once `claims` reference those items the delete violates the
  `claims.item_id` foreign key and the request 500s with `FOREIGN KEY
constraint failed`. The host must finish editing _before_ sharing the link.
  Known issue, not yet fixed — a proper fix needs item IDs preserved across
  edits so claims survive.
