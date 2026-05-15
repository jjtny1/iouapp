# splitit

Split a restaurant bill with friends. Upload a receipt, it's parsed into line
items, friends open a link and claim what they ordered, and each person settles
their share in stablecoin.

## How it works

1. The host signs in and creates a bill, then uploads a photo of the receipt.
2. The receipt is parsed into items, tax, and tip.
3. The host shares a link. Friends open it, enter their name, and tap the items
   they ordered. Shared items are split evenly among everyone who claims them.
4. Tax and tip are prorated by each person's share of the claimed subtotal.
5. Each friend pays their share to the host's wallet address.

## Architecture

- **Backend** — Go (standard-library `net/http`), SQLite for storage. Single
  binary that also serves the built frontend.
- **Frontend** — React + TypeScript + Vite, mobile-first. Built to static
  assets and served by the Go binary in production.
- **Receipt parsing** — `internal/receipt`: a `Parser` interface with a
  `ClaudeParser` (Anthropic vision API) and a `StubParser` fallback used when no
  API key is configured.
- **Payments** — `internal/payment`: a `Provider` interface. `MockProvider`
  simulates settlement with an HTTP 402 payment challenge; an `X402Provider`
  skeleton documents how real on-chain USDC settlement would drop in.
- **Split math** — `internal/split`: a pure, deterministic function. Items
  split evenly among claimers; tax/tip prorated with the largest-remainder
  method so totals reconcile to the exact cent.

## Prerequisites

- Go 1.25+ (the build auto-selects a newer toolchain if needed)
- Node 20+

## Running it

### Development (hot-reload frontend)

```bash
# terminal 1 — API on :8080
SPLITIT_DEV=1 go run ./cmd/server

# terminal 2 — Vite dev server on :5173, proxies /api to :8080
cd web && npm install && npm run dev
```

### Production (single binary)

```bash
cd web && npm install && npm run build   # outputs web/dist
go build -o splitit ./cmd/server
./splitit                                 # serves API + frontend on :8080
```

## Configuration

All configuration is via environment variables:

| Variable                   | Default                 | Purpose                                                                 |
| -------------------------- | ----------------------- | ----------------------------------------------------------------------- |
| `PORT`                     | `8080`                  | HTTP listen port                                                        |
| `SPLITIT_DB`               | `splitit.db`            | SQLite database file path                                               |
| `SPLITIT_BASE_URL`         | `http://localhost:8080` | Base URL used in magic links and share links                            |
| `SPLITIT_DEV`              | unset                   | `1`/`true` enables dev mode (magic link in response, non-Secure cookie) |
| `ANTHROPIC_API_KEY`        | unset                   | Enables real receipt parsing; falls back to a stub                      |
| `SPLITIT_PAYMENT_PROVIDER` | `mock`                  | Payment backend (`mock`; `x402` reserved)                               |

Without `ANTHROPIC_API_KEY` the app uses a sample-receipt stub, so the full
flow is usable without external services. Without `SPLITIT_DEV`, magic links
are only logged server-side (no email delivery is wired up yet).

## Tests

```bash
go test ./...
```

Covers the split algorithm, the receipt-JSON adapter, and an integration suite
that exercises the HTTP API end to end against a temporary database.

## Status

This is a v1 web app. Receipt parsing is live (with a stub fallback); payments
are mocked behind a provider interface, with real x402 / USDC-on-Base
settlement left as the next step.
