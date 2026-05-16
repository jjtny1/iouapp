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
Packages: `internal/{api,auth,db,config,receipt,transcribe,autosplit,payment,split}`.

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
Auto-split transcription (the audio path only) uses the OpenAI Whisper API
and needs `OPENAI_API_KEY`; without it `transcribe.StubTranscriber` returns a
fixed transcript. A typed auto-split prompt needs no transcription at all.

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
- **Auto-split has the same upload constraint** (plus in-browser recording
  needs a real mic). Drive `POST /api/bills/{id}/auto-split` via the API — it
  takes either an `audio` file or a `text` field:
  `curl -b cookies -F 'audio=@clip.m4a;type=audio/m4a' -F 'host_name=Sam' …`
  or `curl -b cookies -F 'text=Maya had the salad…' -F 'host_name=Sam' …`.
  With no API keys the stub transcriber/assigner run, so the whole flow —
  receipt parse, transcription, assignment — is testable offline.
- **Make a test audio clip with macOS `say`.** `say -o /tmp/c.aiff "I had the
burger and an iced tea"` then `afconvert -f m4af -d aac /tmp/c.aiff
/tmp/c.m4a` produces a real `m4a` that Whisper transcribes — handy for
  exercising the auto-split audio path end to end.
- **Running the built Docker image locally** is the closest test to prod.
  Build _native_ — NOT `--platform linux/amd64`, that is only for the Fargate
  push: `docker build -t iou:local .`. Then `docker run --rm -p 8080:8080 -e
IOU_DEV=1 -e IOU_DB=/tmp/iou.db -e ANTHROPIC_API_KEY -e OPENAI_API_KEY
iou:local` — a bare `-e NAME` forwards that var from your shell so keys
  never hit the command line. `IOU_DB` must sit under `/tmp`: the distroless
  `nonroot` user cannot write `/app`, and the file is ephemeral per run.

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
- **Don't try to send audio to the Anthropic API.** It accepts only text,
  images, and PDFs — there is no audio content block. The host's spoken split
  must be transcribed to text first: the app does this with the OpenAI Whisper
  API in `internal/transcribe`, then feeds the transcript to Claude in
  `internal/autosplit`.
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
  `ANTHROPIC_API_KEY` (receipt parsing + auto-split assignment) and
  `OPENAI_API_KEY` (audio transcription) inline on every start, or those
  features silently fall back to their stubs.
- **Don't run with a placeholder or invalid API key.** The stub fallback
  triggers only when the key env var is _empty or unset_. A non-empty but
  bogus key (e.g. pasting `sk-ant-...your-key...` literally) makes the app
  pick the _real_ `ClaudeParser`, which then 401s on every receipt. The user
  just sees a generic "could not parse receipt"; the real cause shows only
  in the server log — `bill receipt: parse: anthropic status 401`. So when a
  receipt won't parse, **check the server log first** for the underlying
  error. Either set a real key or leave the var fully unset — never a
  placeholder.
- **Don't use Node < 20.** The Bash tool snapshots PATH at session start; to use
  Node 20, prefix commands with
  `export NVM_DIR="$HOME/.nvm"; source "$NVM_DIR/nvm.sh"; nvm use 20 >/dev/null 2>&1;`
- **Don't commit straight to `main`.** Branch first, then open a PR.
- **Don't run `npx tsc` (or `npm run build`) in a fresh worktree before
  `npm install`.** A new worktree has no `web/node_modules`; `npx tsc` then
  silently downloads an unrelated registry package (it prints "This is not the
  tsc command you are looking for") and `tsc -b` fails with `tsc: command not
found`. Run `cd web && npm install` first, then type-check with
  `./node_modules/.bin/tsc -p tsconfig.app.json --noEmit` — the local binary,
  not `npx`.
- **Don't rename the Go module path while other worktree branches are in
  flight.** Renaming `module` in `go.mod` rewrites every
  `import "github.com/jjtny1/iouapp/..."` line repo-wide — it's all-or-nothing.
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
- **Don't encode Venmo deep-link params with `url.Values.Encode()` alone.** It
  form-encodes spaces as `+`, and Venmo's deep-link parser renders the `+`
  literally in the payment note (`My+share+of+Cafe…`). Percent-encode spaces as
  `%20` instead — `internal/payment` does
  `strings.ReplaceAll(q.Encode(), "+", "%20")`.
- **Don't put the `venmo.com` web link in the pay QR code.** A phone camera
  scanning an `https://account.venmo.com/pay?…` link opens Venmo's _website_ (a
  login wall), not the app. Encode the `venmo://` app deep link in the QR — the
  camera opens it straight in the Venmo app. The `web_url` is only for paying on
  the desktop machine itself.
- **Don't build the production Docker image without `--platform linux/amd64`.**
  Apple Silicon Macs build arm64 images by default, but the Fargate task runs
  x86_64 — a native-arch image pushes fine, then the task dies on start with an
  `exec format error`. Always `docker build --platform linux/amd64 …`.
- **Deploying from an Apple Silicon Mac: cross-compile — don't run the
  in-Dockerfile amd64 build.** The repo `Dockerfile` compiles Go and the
  frontend _inside_ `golang`/`node` build stages. Building it
  `--platform linux/amd64` on an arm64 Mac runs those `RUN` steps under qemu
  emulation, which reliably **stalls** — the build wedges at 0% CPU with no
  output and never finishes. The fast, reliable path is to cross-compile on
  the host and assemble a COPY-only image (no emulated `RUN` → builds in
  seconds):
  1. `cd web && npm run build` — the frontend is architecture-independent.
  2. `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w"
-o server ./cmd/server` — host Go matches `go.mod` (1.25.0), so the
     binary is identical to the in-container build.
  3. A minimal Dockerfile — `FROM gcr.io/distroless/static:nonroot`,
     `WORKDIR /app`, `COPY server /app/server`, `COPY web/dist /app/web/dist`,
     `EXPOSE 8080`, `USER nonroot:nonroot`, `ENTRYPOINT ["/app/server"]` —
     built `--platform linux/amd64`. Put the binary, `web/dist`, and this
     Dockerfile in a clean temp dir so the repo `.dockerignore` (which strips
     `web/dist`) doesn't empty the build context.
- **A stalled `docker build` is almost always Docker-VM disk pressure.** If a
  build hangs at 0% CPU, run `docker system df` — dead containers and stale
  image/cache layers fill the Docker Desktop VM disk and wedge BuildKit
  mid-build. Clear it with `docker container prune -f && docker image prune
-af && docker builder prune -af`, then rebuild. Killing a stuck
  `docker build` may need `kill -9` — it sometimes ignores SIGTERM, and a
  half-killed build left running concurrently with a retry makes the wedge
  worse.
- **Don't let `terraform apply` create the ECS service before an image exists
  in ECR — actually, it's fine.** `aws_ecs_service` does not wait for task
  health unless `wait_for_steady_state = true` (it isn't set), so a single full
  apply succeeds; the service just has no healthy task until you push an image
  and `update-service --force-new-deployment`. No need to stage the apply.
- **Don't persist a host-split friend's picked identity.** A host-split
  (`split_mode='host'`) share link is one link for the whole group — a shared
  roster. `FriendSplit` must NOT save the "which one are you?" pick to
  `localStorage`: that locks the device to the first person picked, so
  reopening the link always skips straight to them. The pick is session-only;
  the `localStorage` restore on load is gated to the claim flow
  (`split_mode !== 'host'`).
- **Show per-section feedback inside that section, not at the page top.**
  `BillEditor` is a long scrolling page and its shared `error` state renders
  near the top. An action whose control lives in a card far down the page
  (e.g. the auto-split card) must show its own success/failure state _inside
  that card_ — a top-of-page error is off-screen and the action looks like it
  silently did nothing. For the same reason, don't replace the whole editor
  with a full-screen processing view for an in-card action: it resets scroll
  position; run the processing animation inside the card.

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
  flow testable offline. `internal/transcribe` and `internal/autosplit` follow
  the same key-or-stub pattern (`transcribe.New`, `autosplit.New`).
- **Auto-split is an optional host-driven split mode.** A bill's `split_mode`
  is `'claim'` (default — friends self-claim items) or `'host'`. Auto-splitting
  is optional: a bill the host never auto-splits stays a normal claim bill.
  The host describes the split either by **typing a prompt** or by **recording/
  uploading audio**. Audio goes through `internal/transcribe` (Whisper
  `WhisperTranscriber`, else `StubTranscriber`) to become text; a typed prompt
  is used verbatim (no transcription). Either way the text plus the parsed
  items goes to `internal/autosplit` (Claude `ClaudeAssigner`, else
  `StubAssigner`), which maps them onto per-item people — referenced by 1-based
  index, not UUID, so the model can't hallucinate IDs. The endpoint
  `POST /api/bills/{id}/auto-split` (host-only) takes an `audio` file **or** a
  `text` field, creates the named people as `participants`, writes `claims`,
  and stores the text in `bills.split_prompt` — `split.Compute` is unchanged.
  It is re-runnable: every `host_managed` participant and their claims are
  replaced in one transaction.
- **Host-managed participants vs self-joined.** `participants.host_managed`
  flags people the host created via auto-split; `participants.is_host` flags
  the host's own participant (shown for completeness — owes no payment to
  themselves). For a `split_mode='host'` bill `handleJoinBill` rejects new
  joins, and the summary exposes each `participant_token` (gated by the share
  token) so a friend opens the link, picks their name, and pays without ever
  self-claiming. The auto-split editor must run _after_ items are saved —
  editing items afterward hits the `claims` foreign-key issue below.
- **Payments are Venmo hand-offs.** The host saves a `venmo_handle` on their
  user row (set in the bill editor or on the Home page; new tabs reuse it).
  `POST /pay` returns a payment intent — the host's handle, the amount owed,
  and `app_url` (`venmo://`) / `web_url` (`account.venmo.com`) deep links built
  by `internal/payment`. Phones open `app_url` directly; the desktop pay sheet
  shows a QR code that _also_ encodes `app_url` (so a scanning phone lands in
  the Venmo app), with `web_url` only as a click-through for paying on the
  desktop itself. Venmo reports nothing back, so a payment is marked paid by
  the friend's self-report (`POST /pay/confirm`, no proof) or by the host
  toggling it (`POST /bills/{id}/payments/{pid}` with `{"paid":bool}`). The
  `payments` table keeps vestigial `provider`/`tx_ref` columns from the earlier
  USDC design — always written `'venmo'`/`NULL`, never read.
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
- **A claim carries a `share_count` for splitting a shared dish.** `claims`
  has a `share_count` column (default 1): a friend who taps an item declares
  how many ways it's shared with the headcount stepper, and pays `1/N` of it.
  `split.splitItem` gives each claimer an _effective denominator_ of
  `max(share_count, claimer count)`, which is the elegant load-bearing rule:
  it is never below the claimer count, so the item never over-collects and a
  lone first-tapper who sets "3 ways" is charged a third immediately (the rest
  stays unclaimed); when nobody sets a count it collapses to the old implicit
  even split (`max(1, m) == m`). A claimer is never charged more than the
  `1/N` they declared. The split engine takes `[]split.Claim`
  (`{ParticipantID, ShareCount}`), not bare participant IDs. The
  `PUT …/claims` API accepts the current `claims:[{item_id,share_count}]`
  shape and still the legacy `item_ids:[…]` (each an implicit count of 1);
  `share_count` is server-clamped to `[1, 20]`. The `claims` table is
  INSERTed from _two_ places: `handleSetClaims` lists `share_count`
  explicitly, but `autosplit.applyAutoSplit` inserts `(item_id,
participant_id)` only and relies on the column's `DEFAULT 1`. That works
  because a host-assigned (auto-split) claim _is_ a whole-item claim — `1`
  is the semantically correct default. Don't change `share_count`'s default
  or meaning without auditing both INSERT sites.
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
  Terraform state or the task definition. `OPENAI_API_KEY` (auto-split audio
  transcription) lives in SSM the same way, as `/iou/OPENAI_API_KEY`, and is
  injected into the container by ECS (task definition `iou:2` onward).
  `IOU_DEV` is never set in prod. SES
  starts in sandbox mode (only verified recipient addresses receive mail);
  request production access to email arbitrary users.
- **The Terraform state is not in the repo** — no remote backend, and no local
  `terraform.tfstate` on the build machine. Routine redeploys don't need it
  (build amd64 → push ECR → `update-service --force-new-deployment`), but
  changing _managed infra_ does. `OPENAI_API_KEY` was wired in via the AWS CLI
  directly — SSM SecureString `/iou/OPENAI_API_KEY`, the `iou-task-execution`
  IAM policy, and task definition `iou:2` — with `deploy/terraform/` edited to
  match. A future `terraform apply` that recovers state must first
  `terraform import aws_ssm_parameter.openai_api_key /iou/OPENAI_API_KEY`, or
  it conflicts with the already-existing parameter.
- **Don't deploy to prod before the change is merged to `main`.** Build/push
  the image and roll the ECS service only after the PR merges — production
  runs merged code only.
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
