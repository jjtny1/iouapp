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
The same SPA also ships as a native iOS app — a Capacitor wrapper in `web/ios/`
(see "iOS app (Capacitor)" below).

## Commands

| Command                                           | Description                        |
| ------------------------------------------------- | ---------------------------------- |
| `IOU_DEV=1 go run ./cmd/server`                   | Start API on :8080 (dev mode)      |
| `cd web && npm run dev`                           | Start Vite dev server on :5173     |
| `go test ./...`                                   | Run Go tests                       |
| `cd web && npm run build`                         | Build the frontend to `web/dist`   |
| `cd web && npx tsc -p tsconfig.app.json --noEmit` | Type-check the frontend            |
| `cd web && npm run sync:ios`                      | Build the SPA for iOS + sync Xcode |

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

### Browser testing with Chrome

For verifying _rendered_ UI flows (not just the API) with the Chrome
automation tools. Build the SPA first (`cd web && npm run build`) and run the
Go server on an uncommon high port so it serves the current build — the
browser hits the Go server directly, no Vite proxy.

- **Sign in via the dev magic link.** With `IOU_DEV=1` the SignIn page shows a
  "Dev link — open it" link after you submit an email. _Clicking it bounces
  straight back to `/signin`_ — the known `AuthProvider` bootstrap race (see
  Learned Patterns). The cookie is set by then anyway, so just `navigate` to
  `/` and the session sticks. Don't chase the bounce as a bug.
- **Drive the editor by element ref, not pixel coordinates.** The bill editor
  is a long page that scrolls and re-renders after every save / auto-split, so
  cached screenshot coordinates go stale and clicks land between elements
  (silently — no request fires). Use `find` to get a fresh `ref` for the
  field/button, then click or `form_input` against the ref.
- **File uploads can't be driven** (the extension blocks programmatic
  `file_upload`). Skip the receipt-photo and audio-record paths in the
  browser: use the editor's **"Enter items manually"** button and the
  **typed** auto-split prompt instead — both exercise the same backend.
- **Confirm a save actually happened server-side — don't trust the UI.** A
  missed click looks like nothing happened. After an action, `grep` the server
  log for the expected `PATCH`/`POST` line and inspect the SQLite DB
  (`IOU_DB`) directly; `grep -i 'error|constraint|500|panic'` the log to catch
  a 500 the UI swallowed into a generic message. `read_console_messages` with
  `onlyErrors` catches frontend exceptions.
- A full claim-aware flow with no API keys: create tab → "Enter items
  manually" → "Save & continue" (lands on the Split step) → "I'll split it up"
  → typed auto-split (StubAssigner names the people) → "Continue" → Share step.
  To re-test editing a bill that already has claims, jump back to **Review** via
  the step bar, edit an item, and "Save & continue" again — that save is the
  regression check for editing a bill with existing claims.

---

## iOS app (Capacitor)

The SPA also ships as a **native iOS app** — a Capacitor 7 wrapper around the
same Vite build, Xcode project in `web/ios/`. Capacitor 7, _not_ 8: v8 needs
Node 22 and the project is pinned to Node 20. CocoaPods is required
(`brew install cocoapods`).

- **Build & run**: `cd web && npm run sync:ios` builds the SPA against the live
  API and copies it into the Xcode project; then open
  `web/ios/App/App.xcworkspace` and Run. `npm run build:ios` is the build step
  alone.
- **The native app bundles a frozen snapshot of the web build** at
  `web/ios/App/App/public/` — it does NOT auto-update from prod the way the
  website does. After any web change lands, run `npm run sync:ios` and rebuild
  in Xcode, or the app keeps serving stale web code. If a sync doesn't seem to
  take, Xcode is caching the bundle — Product → Clean Build Folder.
- **The native app is cross-origin with the API** — the WebView is served from
  `capacitor://localhost`, the API is `https://iouapp.ai`. Three consequences,
  all already wired: the SPA's API base is `VITE_API_BASE` (empty for the web
  build → relative `/api`; set to the live URL by `build:ios`); the Go backend
  has a `cors` middleware allowing the `capacitor://localhost` origin; and the
  native build authenticates with an `Authorization: Bearer` token, since the
  cross-site session cookie isn't sent — `verify` returns the token,
  `currentUser`/`logout` also accept the bearer header. The web app is
  unchanged: same-origin, cookie auth.
- **Universal Links** carry magic-link sign-in into the app. The server
  publishes `/.well-known/apple-app-site-association` when `IOU_APPLE_APP_ID`
  (`<TeamID>.ai.iouapp.app`) is set; `DeepLinkHandler` in `App.tsx` routes the
  Capacitor `appUrlOpen` event to the SPA's `/auth/verify` route. For dev the
  entitlement is `applinks:iouapp.ai?mode=developer` — it bypasses Apple's CDN,
  which negative-caches a failed AASA fetch for an hour, and needs the device
  toggle Settings → Developer → Associated Domains Development. **Revert it to
  plain `applinks:iouapp.ai` before an App Store build.**
- **Don't let content render under the status bar.** The Capacitor WebView is
  full-screen; `index.html` sets `viewport-fit=cover` and `.paper-app` pads
  with `env(safe-area-inset-*)` so content clears the notch / home indicator.
  These insets are 0 in a desktop browser, so the web layout is unaffected.
- **Don't enlarge the receipt-editor inputs to stop iOS focus-zoom.** iOS
  auto-zooms into any focused input under 16px, and an app-switch mid-zoom
  (tapping the magic-link email) leaves the WebView stuck zoomed in. The
  receipt inputs are deliberately small, so instead `main.tsx` disables
  WebView zoom on native only (`maximum-scale=1, user-scalable=no`, gated by
  `Capacitor.isNativePlatform()`), leaving the web app zoomable.

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
  literally in the payment note (`My+share+of+Cafe…`). Percent-encode spaces
  as `%20` instead — `internal/payment` does
  `strings.ReplaceAll(q.Encode(), "+", "%20")`.
- **Don't put the `venmo.com` web link in the pay QR code.** A phone camera
  scanning an `https://account.venmo.com/pay?…` link opens Venmo's _website_
  (a login wall), not the app. Encode the `venmo://` app deep link in the QR
  — the camera opens it straight in the Venmo app. The `web_url` is only for
  paying on the desktop machine itself.
- **Don't migrate to the `https://venmo.com/<handle>?…` Universal Link.** It
  has a note display bug: its renderer shows BOTH `+` and `%20` as a literal
  `+` between every word of the prefilled note (`My+share+of+Cafe…`).
  Confirmed Venmo-side by pasting the URL straight into Safari and letting it
  open Venmo — not our server, not the SPA. History: PR #28 made the switch
  after a beta tester saw "We don't recognize that code. Recheck and try
  again." from the `venmo://paycharge` deeplink; PR #29 (%20) and a follow-up
  NBSP attempt both shipped visible regressions. PR #32 reverted to the
  deeplink — Venmo had since fixed the "we don't recognize" error, so the
  deeplink (with its existing `+` → `%20` workaround) is the working format
  as of 2026-05-19. If "we don't recognize" comes back AND the Universal
  Link's note bug is still present, drop the note from the URL entirely
  rather than fight either encoding bug.
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
- **Don't drop the totalbar's repaint workarounds.** iOS Safari has a
  position:sticky compositing bug: when only the inner _text_ of a sticky
  element with a `box-shadow` changes, the layer isn't marked dirty, so the
  new value sits in the DOM but doesn't paint until the user scrolls. Beta
  tester surfaced this against `FriendSplit`'s "You owe" total after the
  optimistic-claims change made the value update faster than a scroll. Two
  belt-and-braces fixes both load-bearing:
  1. `.totalbar` in `web/src/index.css` has `transform: translate3d(0,0,0)`
     to promote it to its own compositing layer (iOS flushes the layer on
     content updates that way).
  2. The `<p className="amt">` in `FriendSplit.tsx` has `key={owes}` so React
     unmounts/remounts the node on every value change — guarantees a DOM
     swap iOS can't skip painting.
     Either one alone may be enough, but both together survive future React /
     CSS edits that might invalidate the other. Symptom to recognise: "value
     only updates when I scroll."

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
  and `app_url` / `web_url` (both the same `https://venmo.com/<handle>?txn=pay…`
  Universal Link, since Venmo broke the legacy `venmo://paycharge?…` scheme —
  see the matching note in Mistakes to Avoid). The two fields stay separate in
  the API for the existing intent shape. Phones open `app_url` directly (iOS /
  Android route the Universal Link into the app); the desktop pay sheet shows a
  QR code that encodes `app_url` (a scanning phone follows it into the app);
  `web_url` is the click-through for paying on the desktop itself. Venmo reports nothing back, so a payment is marked paid by
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
- **`bills.status` is vestigial — always `'draft'`.** The column exists
  (`schema.sql` defaults it to `'draft'`) and `PATCH /api/bills/{id}` still
  accepts a `status` of `'draft'`/`'open'`, but nothing in the app ever
  transitions it: not the editor's save, not a friend joining, not a payment.
  The frontend doesn't send or display it. Treat it like the `payments`
  table's `provider`/`tx_ref` columns — left in place, never read. Don't build
  UI on `status` (the old Home page split tabs into "Open"/"Settled" on a
  `status === 'settled'` that was never written) without first wiring a real
  transition end to end.
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
- **`FriendSplit` claim updates are optimistic, with a request counter to
  resolve out-of-order responses.** `toggleItem` / `setShareCount` mutate a
  fresh claims `Map` and call `saveClaims`, which stashes the map in a
  `pendingClaims` state _before_ awaiting the API. The checkboxes, per-item
  denominators, and the "You owe" total all read from `pendingClaims` when
  it's set (else the live `summary`), so a tap shows immediately instead of
  waiting on the round-trip. `saveReqRef` (a `useRef` counter) is bumped on
  every save; the response handler only applies `setSummary` /
  `setPendingClaims(null)` if its `myReq === saveReqRef.current`, so an
  older save returning out of order can't clobber a newer summary. The
  optimistic "you owe" number is computed by a helper `optimisticShare`
  that mirrors `internal/split.Compute`'s proration (tax / tip and a
  percent service charge scale with the friend's item subtotal) but skips
  the largest-remainder pennies — the delta from the eventual server value
  is ≤ 1¢ and snaps into place when the response lands. A fixed service
  charge stays on the live summary's value since it splits by headcount,
  not by claims. The previous synchronous `await api.setClaims → setSummary`
  flow shipped a visible-wrong intermediate render AND had a small
  fast-double-tap race that dropped the first claim — both are gone with
  this pattern. (Pairs with the iOS sticky repaint workaround in Mistakes
  to Avoid — without it the new optimistic value still wouldn't paint.)
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
- **`saveBillAndItems` reconciles items by id, not delete-and-recreate.** The
  editor sends each item's `id` back on save (`PATCH /api/bills/{id}`); the
  server updates matching rows in place, inserts items with no/unknown id as
  new, and deletes dropped items after first clearing any `claims` on them. A
  kept item keeps its id, so a `claims.item_id` row survives an edit — the
  host can edit a bill after friends have claimed. (The earlier code deleted
  and recreated every `items` row, which 500'd with `FOREIGN KEY constraint
failed` once claims existed.) Receipt re-upload still sends items with no id,
  so it replaces every item and cascade-clears their claims — re-parsing a
  receipt intentionally starts the split over.
- **`BillEditor` is a four-step flow, not one long page.** A `step` state
  (`"review" | "split" | "share"`) plus a `StepBar` walk the host through
  Receipt → Review → Split → Share. The receipt upload/parse screens are step 1
  (`Receipt`); the post-parse editor renders one step at a time. `Review` edits
  the items and its only button, `saveAndContinue`, calls `onSave` then
  advances — `onSave` returns a `boolean` so the advance is gated on a
  successful save. `Split` opens on a **choice** (`splitView` state): "I'll
  split it up" reveals the auto-split form, "My friends will choose" jumps
  straight to `Share`. `Share` holds the Venmo handle, share link and Joined
  list. The `StepBar` lets the host jump back to any reached step; steps past
  Review need a saved bill (`bill.items.length > 0`). Don't reintroduce a
  single-scroll editor — the stepped flow is the fix for testers finding the
  old page confusing.
- **Processing animations have a minimum on-screen time.** `ParsingView` and
  `AutoSplitView` step through their messages **once and hold** on the last one
  (no looping — looping read as frantic), and the calling handler holds the
  animation a minimum ~1.6–1.8s via `holdFor(startedAt, min)` so a fast or
  stubbed response doesn't flash the loading state past in a blink. Any new
  processing/loading state should follow the same hold-and-don't-loop pattern.
- **The `Brand` wordmark can double as a "back" affordance.** `Brand` (in
  `ui.tsx`) takes an optional `onClick`; passing it renders the wordmark as a
  button. `FriendSplit`'s settled-up screen uses this so the otherwise-terminal
  "you're square" page isn't a dead end — the logo steps the friend back to
  their split view (`showSplit` state toggles the `isPaid` early return).
